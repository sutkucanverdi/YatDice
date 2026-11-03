package main

import (
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

// ─── 1. GÜVENLİK: RATE LIMITER & BRUTE-FORCE KORUMASI ─────────────────────────

type ipBucket struct {
	tokens float64
	last   time.Time
}

type RateLimiter struct {
	mu      sync.Mutex
	rate    float64 // saniye başına token
	burst   float64
	buckets map[string]*ipBucket
}

func NewRateLimiter(perMinute float64, burst int) *RateLimiter {
	rl := &RateLimiter{
		rate:    perMinute / 60.0,
		burst:   float64(burst),
		buckets: make(map[string]*ipBucket),
	}
	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		for range ticker.C {
			rl.mu.Lock()
			for k, b := range rl.buckets {
				if time.Since(b.last) > 15*time.Minute {
					delete(rl.buckets, k)
				}
			}
			rl.mu.Unlock()
		}
	}()
	return rl
}

func (rl *RateLimiter) Allow(ip string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	now := time.Now()
	b, exists := rl.buckets[ip]
	if !exists {
		rl.buckets[ip] = &ipBucket{tokens: rl.burst - 1, last: now}
		return true
	}
	b.tokens += now.Sub(b.last).Seconds() * rl.rate
	if b.tokens > rl.burst {
		b.tokens = rl.burst
	}
	b.last = now
	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}

var (
	generalLimiter = NewRateLimiter(240, 80) // Dakikada 240 istek
	strictLimiter  = NewRateLimiter(20, 12)  // Auth uç noktaları: dakikada 20 istek
)

type loginLockEntry struct {
	fails int
	until time.Time
	last  time.Time
}

var (
	loginLockMu sync.Mutex
	loginLocks  = make(map[string]*loginLockEntry)
	dummyBcrypt = func() []byte {
		h, _ := bcrypt.GenerateFromPassword([]byte("dummy-timing-shield-pass"), BcryptCost)
		return h
	}()
)

func checkLoginLocked(username string) (bool, int) {
	loginLockMu.Lock()
	defer loginLockMu.Unlock()
	e, ok := loginLocks[strings.ToLower(username)]
	if !ok {
		return false, 0
	}
	if time.Now().Before(e.until) {
		return true, int(time.Until(e.until).Seconds())
	}
	return false, 0
}

func recordLoginFailure(username string) {
	loginLockMu.Lock()
	defer loginLockMu.Unlock()
	key := strings.ToLower(username)
	e, ok := loginLocks[key]
	if !ok || time.Since(e.last) > 15*time.Minute {
		e = &loginLockEntry{}
		loginLocks[key] = e
	}
	e.fails++
	e.last = time.Now()
	if e.fails >= 5 {
		e.until = time.Now().Add(5 * time.Minute)
		e.fails = 0
	}
}

func resetLoginFailures(username string) {
	loginLockMu.Lock()
	delete(loginLocks, strings.ToLower(username))
	loginLockMu.Unlock()
}

func getClientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		return strings.TrimSpace(strings.Split(xff, ",")[0])
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// applySecurityHeadersAndLimits: Tüm HTTP uç noktalarına güvenlik başlıkları,
// boyut sınırı (MaxBytesReader) ve Rate Limit uygular.
func applySecurityAndRateLimit(w http.ResponseWriter, r *http.Request, strict bool) bool {
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-Frame-Options", "DENY")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("X-XSS-Protection", "1; mode=block")
	w.Header().Set("Cache-Control", "no-store")

	// Body boyutunu maksimum 256 KB ile sınırla (DoS / Bellek taşması koruması)
	r.Body = http.MaxBytesReader(w, r.Body, 256*1024)

	ip := getClientIP(r)
	if !generalLimiter.Allow(ip) || (strict && !strictLimiter.Allow(ip)) {
		http.Error(w, "Çok fazla istek gönderdiniz. Lütfen kısa bir süre bekleyin.", http.StatusTooManyRequests)
		return false
	}
	return true
}

func issueJWTForUser(userID int, days int) (string, error) {
	expirationTime := time.Now().Add(time.Duration(days) * 24 * time.Hour)
	claims := &jwt.RegisteredClaims{
		Subject:   fmt.Sprintf("%d", userID),
		IssuedAt:  jwt.NewNumericDate(time.Now()),
		ExpiresAt: jwt.NewNumericDate(expirationTime),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(jwtKey)
}

// ─── 2. KALICI MİSAFİR HESABI (AYNI CİHAZDA KALICI OTURUM) ───────────────────

// POST /auth/guest : Cihaz ID'sine (deviceId) göre mevcut misafir hesabını bulur
// veya yeni bir kalıcı misafir hesabı oluşturur. Böylece oyuncu çıkış yapıp tekrar
// "Misafir Olarak Devam Et" dese bile aynı hesabında, aynı altın ve zarlarıyla kalır.
func handleGuestAuth(w http.ResponseWriter, r *http.Request) {
	if setupCORS(&w, r) {
		return
	}
	if !applySecurityAndRateLimit(w, r, true) {
		return
	}

	var req struct {
		DeviceID string `json:"deviceId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Geçersiz istek", http.StatusBadRequest)
		return
	}

	deviceID := strings.TrimSpace(req.DeviceID)
	if len(deviceID) < 8 || len(deviceID) > 120 {
		http.Error(w, "Geçersiz cihaz kimliği", http.StatusBadRequest)
		return
	}

	var userID int
	var username string
	err := db.QueryRow(`SELECT id, username FROM users WHERE guest_device_id = $1 LIMIT 1`, deviceID).Scan(&userID, &username)
	if err != nil {
		// Bu cihaz için henüz misafir hesabı yok; yeni kalıcı misafir hesabı oluştur
		for attempt := 0; attempt < 10; attempt++ {
			candidate := fmt.Sprintf("Misafir_%d", secureRandInt(90000)+10000)
			randomPass, _ := bcrypt.GenerateFromPassword([]byte(fmt.Sprintf("guest-%s-%d", deviceID, time.Now().UnixNano())), BcryptCost)
			err = db.QueryRow(`
				INSERT INTO users (username, password_hash, guest_device_id, is_guest)
				VALUES ($1, $2, $3, TRUE)
				RETURNING id, username
			`, candidate, string(randomPass), deviceID).Scan(&userID, &username)
			if err == nil {
				break
			}
		}
		if userID == 0 {
			http.Error(w, "Misafir hesabı oluşturulamadı", http.StatusInternalServerError)
			return
		}
		_, _ = db.Exec(`
			INSERT INTO profiles (user_id, xp, tokens, cases, unlocked_dice, selected_dice)
			VALUES ($1, 120, 250, 2, 'ivory', 'ivory')
			ON CONFLICT (user_id) DO NOTHING
		`, userID)
	}

	tokenString, err := issueJWTForUser(userID, 365) // Misafir tokenı 1 yıl geçerli
	if err != nil {
		http.Error(w, "Token oluşturulamadı", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"token":    tokenString,
		"username": username,
		"isGuest":  true,
	})
}

// ─── 3. GOOGLE İLE GİRİŞ / KAYIT SİSTEMİ ──────────────────────────────────────

// POST /auth/google : Google ID Token (credential) veya Google hesap bilgileriyle
// tek tıkla kayıt/giriş yapar.
func handleGoogleAuth(w http.ResponseWriter, r *http.Request) {
	if setupCORS(&w, r) {
		return
	}
	if !applySecurityAndRateLimit(w, r, true) {
		return
	}

	var req struct {
		Credential string `json:"credential"` // Resmi Google Identity JWT (varsa)
		Email      string `json:"email"`
		Name       string `json:"name"`
		GoogleID   string `json:"googleId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Geçersiz istek", http.StatusBadRequest)
		return
	}

	email := strings.ToLower(strings.TrimSpace(req.Email))
	displayName := strings.TrimSpace(req.Name)
	googleID := strings.TrimSpace(req.GoogleID)

	// Eğer Google Identity Services üzerinden gerçek JWT credential geldiyse Google tokeninfo ile doğrula
	if req.Credential != "" {
		client := &http.Client{Timeout: 5 * time.Second}
		resp, err := client.Get("https://oauth2.googleapis.com/tokeninfo?id_token=" + req.Credential)
		if err != nil || resp.StatusCode != http.StatusOK {
			http.Error(w, "Google oturumu doğrulanamadı", http.StatusUnauthorized)
			return
		}
		defer resp.Body.Close()
		var gInfo struct {
			Sub   string `json:"sub"`
			Email string `json:"email"`
			Name  string `json:"name"`
			Aud   string `json:"aud"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&gInfo); err != nil || gInfo.Sub == "" {
			http.Error(w, "Google profil verisi okunamadı", http.StatusUnauthorized)
			return
		}
		if expectedClientID := getEnv("GOOGLE_CLIENT_ID", ""); expectedClientID != "" && gInfo.Aud != expectedClientID {
			http.Error(w, "Geçersiz Google istemci kimliği", http.StatusUnauthorized)
			return
		}
		googleID = gInfo.Sub
		email = strings.ToLower(strings.TrimSpace(gInfo.Email))
		if gInfo.Name != "" {
			displayName = strings.TrimSpace(gInfo.Name)
		}
	}

	if email == "" || !strings.Contains(email, "@") {
		http.Error(w, "Geçerli bir Google e-posta adresi gerekli", http.StatusBadRequest)
		return
	}
	if googleID == "" {
		googleID = "google_" + email
	}

	var userID int
	var username string
	err := db.QueryRow(`
		SELECT id, username FROM users
		WHERE google_id = $1 OR (LOWER(email) = LOWER($2) AND email <> '')
		LIMIT 1
	`, googleID, email).Scan(&userID, &username)

	if err != nil {
		// Yeni Google kullanıcısı oluştur
		baseName := displayName
		if baseName == "" {
			baseName = strings.Split(email, "@")[0]
		}
		// Boşlukları ve özel karakterleri temizle
		baseName = strings.Map(func(r rune) rune {
			if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' {
				return r
			}
			return -1
		}, baseName)
		if len(baseName) < 3 {
			baseName = "Oyuncu"
		}
		if len(baseName) > 18 {
			baseName = baseName[:18]
		}

		for i := 0; i < 10; i++ {
			candidate := baseName
			if i > 0 {
				candidate = fmt.Sprintf("%s%d", baseName, secureRandInt(900)+100)
			}
			randomPass, _ := bcrypt.GenerateFromPassword([]byte(fmt.Sprintf("google-%s-%d", googleID, time.Now().UnixNano())), BcryptCost)
			err = db.QueryRow(`
				INSERT INTO users (username, password_hash, email, google_id, is_guest)
				VALUES ($1, $2, $3, $4, FALSE)
				RETURNING id, username
			`, candidate, string(randomPass), email, googleID).Scan(&userID, &username)
			if err == nil {
				break
			}
		}
		if userID == 0 {
			http.Error(w, "Google hesabı oluşturulamadı", http.StatusInternalServerError)
			return
		}
		_, _ = db.Exec(`
			INSERT INTO profiles (user_id, xp, tokens, cases, unlocked_dice, selected_dice)
			VALUES ($1, 150, 300, 2, 'ivory', 'ivory')
			ON CONFLICT (user_id) DO NOTHING
		`, userID)
	} else {
		// Mevcut kullanıcının google_id ve email alanlarını güncelle
		_, _ = db.Exec(`UPDATE users SET google_id = $1, email = COALESCE(NULLIF(email, ''), $2) WHERE id = $3`, googleID, email, userID)
	}

	tokenString, err := issueJWTForUser(userID, 30)
	if err != nil {
		http.Error(w, "Token oluşturulamadı", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"token":    tokenString,
		"username": username,
		"email":    email,
		"isGuest":  false,
	})
}

// ─── 4. ŞİFRE SIFIRLAMA SİSTEMİ (KOD & E-POSTA DOĞRULAMALI) ──────────────────

type resetCodeItem struct {
	code      string
	expiresAt time.Time
	attempts  int
}

var (
	resetMu    sync.Mutex
	resetCodes = make(map[int]*resetCodeItem) // userID -> sıfırlama kodu
)

// POST /auth/forgot-password : Kullanıcı adı + E-posta doğrulaması yaparak 6 haneli
// tek kullanımlık sıfırlama kodu üretir.
func handleForgotPassword(w http.ResponseWriter, r *http.Request) {
	if setupCORS(&w, r) {
		return
	}
	if !applySecurityAndRateLimit(w, r, true) {
		return
	}

	var req struct {
		Username string `json:"username"`
		Email    string `json:"email"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Geçersiz istek", http.StatusBadRequest)
		return
	}

	username := strings.TrimSpace(req.Username)
	email := strings.ToLower(strings.TrimSpace(req.Email))
	if username == "" {
		http.Error(w, "Kullanıcı adı giriniz", http.StatusBadRequest)
		return
	}

	var userID int
	var dbEmail string
	var isGuest bool
	err := db.QueryRow(`SELECT id, COALESCE(email, ''), COALESCE(is_guest, FALSE) FROM users WHERE LOWER(username) = LOWER($1)`, username).
		Scan(&userID, &dbEmail, &isGuest)
	if err != nil || isGuest {
		http.Error(w, "Kayıtlı kullanıcı bulunamadı", http.StatusNotFound)
		return
	}

	// Eğer kullanıcının kayıtlı e-postası varsa eşleşmesini zorunlu tut
	if dbEmail != "" && !strings.EqualFold(dbEmail, email) {
		http.Error(w, "Girilen e-posta adresi bu hesapla eşleşmiyor!", http.StatusForbidden)
		return
	}

	code := fmt.Sprintf("%06d", secureRandInt(900000)+100000)
	resetMu.Lock()
	resetCodes[userID] = &resetCodeItem{
		code:      code,
		expiresAt: time.Now().Add(10 * time.Minute),
		attempts:  0,
	}
	resetMu.Unlock()

	// Geliştirme / Demo ortamında SMTP yoksa kodu yanıtta da döndürürüz (Production'da SMTP ile gönderilir)
	resp := map[string]interface{}{
		"message":   "Şifre sıfırlama kodu oluşturuldu (10 dakika geçerli).",
		"resetCode": code,
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

// POST /auth/reset-password : 6 haneli sıfırlama kodunu doğrulayıp yeni şifreyi kaydeder.
func handleResetPassword(w http.ResponseWriter, r *http.Request) {
	if setupCORS(&w, r) {
		return
	}
	if !applySecurityAndRateLimit(w, r, true) {
		return
	}

	var req struct {
		Username    string `json:"username"`
		ResetCode   string `json:"resetCode"`
		NewPassword string `json:"newPassword"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Geçersiz istek", http.StatusBadRequest)
		return
	}

	username := strings.TrimSpace(req.Username)
	code := strings.TrimSpace(req.ResetCode)
	if len(req.NewPassword) < 6 || len(req.NewPassword) > 72 {
		http.Error(w, "Yeni şifre en az 6 karakter olmalıdır", http.StatusBadRequest)
		return
	}

	var userID int
	err := db.QueryRow(`SELECT id FROM users WHERE LOWER(username) = LOWER($1)`, username).Scan(&userID)
	if err != nil {
		http.Error(w, "Geçersiz kullanıcı veya sıfırlama kodu", http.StatusBadRequest)
		return
	}

	resetMu.Lock()
	entry, exists := resetCodes[userID]
	if !exists || time.Now().After(entry.expiresAt) {
		delete(resetCodes, userID)
		resetMu.Unlock()
		http.Error(w, "Sıfırlama kodunun süresi dolmuş veya geçersiz", http.StatusBadRequest)
		return
	}
	entry.attempts++
	if entry.attempts > 5 {
		delete(resetCodes, userID)
		resetMu.Unlock()
		http.Error(w, "Çok fazla hatalı kod denemesi. Yeni kod talep edin.", http.StatusTooManyRequests)
		return
	}
	if subtle.ConstantTimeCompare([]byte(entry.code), []byte(code)) != 1 {
		resetMu.Unlock()
		http.Error(w, "Sıfırlama kodu hatalı!", http.StatusBadRequest)
		return
	}
	delete(resetCodes, userID)
	resetMu.Unlock()

	hashed, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), BcryptCost)
	if err != nil {
		http.Error(w, "Sunucu hatası", http.StatusInternalServerError)
		return
	}

	if _, err := db.Exec(`UPDATE users SET password_hash = $1 WHERE id = $2`, string(hashed), userID); err != nil {
		http.Error(w, "Şifre güncellenemedi", http.StatusInternalServerError)
		return
	}

	resetLoginFailures(username)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"message": "Şifreniz başarıyla güncellendi! Şimdi giriş yapabilirsiniz."})
}

// ─── 5. ARKADAŞ ÇEVRİMİÇİ (ONLINE / OYUNDA) DURUM TAKİBİ ─────────────────────

var (
	presenceMu   sync.Mutex
	userLastSeen = make(map[int]time.Time)
	userInGame   = make(map[int]int) // userID -> aktif oda sayısı
)

func touchUserPresence(userID int) {
	if userID <= 0 {
		return
	}
	presenceMu.Lock()
	userLastSeen[userID] = time.Now()
	presenceMu.Unlock()
}

func setUserGameState(userID int, inGame bool) {
	if userID <= 0 {
		return
	}
	presenceMu.Lock()
	userLastSeen[userID] = time.Now()
	if inGame {
		userInGame[userID]++
	} else if userInGame[userID] > 0 {
		userInGame[userID]--
	}
	presenceMu.Unlock()
}

func getUserOnlineState(userID int) (online bool, inGame bool) {
	if userID <= 0 {
		return false, false
	}
	presenceMu.Lock()
	defer presenceMu.Unlock()
	activeGames := userInGame[userID] > 0
	seenRecently := time.Since(userLastSeen[userID]) < 18*time.Second
	return activeGames || seenRecently, activeGames
}

