package main

import (
	"crypto/subtle"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"mime"
	"net"
	"net/http"
	"net/mail"
	"net/smtp"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"
)

const (
	minAdminTokenLength = 32
	maxChatRecords      = 20
	chatRecordLifetime  = 30 * time.Minute
)

// ─── ENGELLEME ────────────────────────────────────────────────────────────────

func isBlockedEither(a, b int) bool {
	if a <= 0 || b <= 0 {
		return false
	}
	var blocked bool
	err := db.QueryRow(`
		SELECT EXISTS(
			SELECT 1 FROM user_blocks
			WHERE (blocker_id = $1 AND blocked_id = $2) OR (blocker_id = $2 AND blocked_id = $1)
		)
	`, a, b).Scan(&blocked)
	if err != nil {
		log.Println("Engel kontrolü başarısız:", err)
		return true // Hata anında iki kullanıcıyı buluşturmamak daha güvenli
	}
	return blocked
}

func areFriends(a, b int) bool {
	var friends bool
	err := db.QueryRow(`
		SELECT EXISTS(
			SELECT 1 FROM friendships
			WHERE status = 'accepted'
			  AND ((requester_id = $1 AND addressee_id = $2) OR (requester_id = $2 AND addressee_id = $1))
		)
	`, a, b).Scan(&friends)
	return err == nil && friends
}

func userIDByName(username string) (int, string, error) {
	return resolveHandle(db, username)
}

// POST /ugc/block : Oyuncuyu engeller ve varsa arkadaşlığı kaldırır
func handleBlockUser(w http.ResponseWriter, r *http.Request) {
	if setupCORS(&w, r) {
		return
	}
	if !applySecurityAndRateLimit(w, r, false) {
		return
	}
	uid, ok := requireUser(w, r)
	if !ok {
		return
	}

	var req struct {
		Username string `json:"username"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpError(w, r, "Geçersiz istek", http.StatusBadRequest)
		return
	}
	targetID, targetName, err := userIDByName(req.Username)
	if err != nil {
		httpError(w, r, "Oyuncu bulunamadı", http.StatusNotFound)
		return
	}
	if targetID == uid {
		httpError(w, r, "Kendinizi engelleyemezsiniz", http.StatusBadRequest)
		return
	}

	if _, err := db.Exec(`
		INSERT INTO user_blocks (blocker_id, blocked_id, blocked_username)
		VALUES ($1, $2, $3)
		ON CONFLICT (blocker_id, blocked_username) DO UPDATE SET blocked_id = EXCLUDED.blocked_id
	`, uid, targetID, targetName); err != nil {
		httpError(w, r, "Oyuncu engellenemedi", http.StatusInternalServerError)
		return
	}
	_, _ = db.Exec(`
		DELETE FROM friendships
		WHERE (requester_id = $1 AND addressee_id = $2) OR (requester_id = $2 AND addressee_id = $1)
	`, uid, targetID)
	removeInvitesBetween(uid, targetID)

	writeJSON(w, map[string]string{"message": targetName + " engellendi."})
}

// POST /ugc/unblock : Engeli kaldırır
func handleUnblockUser(w http.ResponseWriter, r *http.Request) {
	if setupCORS(&w, r) {
		return
	}
	if !applySecurityAndRateLimit(w, r, false) {
		return
	}
	uid, ok := requireUser(w, r)
	if !ok {
		return
	}
	var req struct {
		UserID int `json:"userId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpError(w, r, "Geçersiz istek", http.StatusBadRequest)
		return
	}
	if _, err := db.Exec(`DELETE FROM user_blocks WHERE blocker_id = $1 AND blocked_id = $2`, uid, req.UserID); err != nil {
		httpError(w, r, "Engel kaldırılamadı", http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]string{"status": "ok"})
}

// GET /ugc/blocks : Kullanıcının engellediği oyuncuları döner
func handleListBlocks(w http.ResponseWriter, r *http.Request) {
	if setupCORS(&w, r) {
		return
	}
	if !applySecurityAndRateLimit(w, r, false) {
		return
	}
	uid, ok := requireUser(w, r)
	if !ok {
		return
	}
	rows, err := db.Query(`
		SELECT u.id, u.username
		FROM user_blocks b
		JOIN users u ON u.id = b.blocked_id
		WHERE b.blocker_id = $1
		ORDER BY b.id DESC
	`, uid)
	if err != nil {
		httpError(w, r, "Engel listesi alınamadı", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	type blockedUser struct {
		UserID   int    `json:"userId"`
		Username string `json:"username"`
	}
	blocked := []blockedUser{}
	for rows.Next() {
		var item blockedUser
		if rows.Scan(&item.UserID, &item.Username) == nil {
			blocked = append(blocked, item)
		}
	}
	writeJSON(w, map[string]interface{}{"blocked": blocked})
}

func removeInvitesBetween(a, b int) {
	mu.Lock()
	defer mu.Unlock()
	for _, pair := range [][2]int{{a, b}, {b, a}} {
		filtered := []GameInvite{}
		for _, inv := range pendingInvites[pair[0]] {
			if inv.FromUserID != pair[1] {
				filtered = append(filtered, inv)
			}
		}
		pendingInvites[pair[0]] = filtered
	}
}

// ─── ŞİKÂYET KANITI ───────────────────────────────────────────────────────────

type chatRecord struct {
	Text string    `json:"text"`
	At   time.Time `json:"at"`
}

var (
	chatRecordsMu sync.Mutex
	chatRecords   = make(map[int][]chatRecord)
)

// Küfür filtresinden önceki özgün metin yalnızca şikâyet kanıtı için kısa süre bellekte tutulur
func recordChatMessage(userID int, text string) {
	if userID <= 0 || text == "" {
		return
	}
	chatRecordsMu.Lock()
	defer chatRecordsMu.Unlock()
	records := append(chatRecords[userID], chatRecord{Text: text, At: time.Now()})
	if len(records) > maxChatRecords {
		records = records[len(records)-maxChatRecords:]
	}
	chatRecords[userID] = records
}

func recentChatEvidence(userID int) string {
	chatRecordsMu.Lock()
	defer chatRecordsMu.Unlock()
	cutoff := time.Now().Add(-chatRecordLifetime)
	recent := []chatRecord{}
	for _, record := range chatRecords[userID] {
		if record.At.After(cutoff) {
			recent = append(recent, record)
		}
	}
	encoded, err := json.Marshal(recent)
	if err != nil {
		return "[]"
	}
	return string(encoded)
}

func pruneChatRecords() {
	chatRecordsMu.Lock()
	defer chatRecordsMu.Unlock()
	cutoff := time.Now().Add(-chatRecordLifetime)
	for userID, records := range chatRecords {
		if len(records) == 0 || records[len(records)-1].At.Before(cutoff) {
			delete(chatRecords, userID)
		}
	}
}

// POST /ugc/report : Oyuncuyu son sohbet kayıtlarıyla birlikte moderasyona bildirir
func handleReportUser(w http.ResponseWriter, r *http.Request) {
	if setupCORS(&w, r) {
		return
	}
	if !applySecurityAndRateLimit(w, r, true) {
		return
	}
	reporterID, ok := requireUser(w, r)
	if !ok {
		return
	}

	var req struct {
		Username string `json:"username"`
		Reason   string `json:"reason"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpError(w, r, "Geçersiz istek", http.StatusBadRequest)
		return
	}
	reason := strings.TrimSpace(req.Reason)
	if reason == "" {
		httpError(w, r, "Bildirim nedeni zorunludur", http.StatusBadRequest)
		return
	}
	if runes := []rune(reason); len(runes) > 250 {
		reason = string(runes[:250])
	}
	targetID, targetName, err := userIDByName(req.Username)
	if err != nil {
		httpError(w, r, "Oyuncu bulunamadı", http.StatusNotFound)
		return
	}
	if targetID == reporterID {
		httpError(w, r, "Kendinizi bildiremezsiniz", http.StatusBadRequest)
		return
	}

	if _, err := db.Exec(`
		INSERT INTO user_reports (reporter_id, reported_user_id, reported_username, reason, evidence)
		VALUES ($1, $2, $3, $4, $5)
	`, reporterID, targetID, targetName, reason, reportEvidence(targetID, reporterID)); err != nil {
		httpError(w, r, "Bildirim kaydedilemedi", http.StatusInternalServerError)
		return
	}

	if notifyTo := strings.TrimSpace(os.Getenv("REPORT_NOTIFY_EMAIL")); notifyTo != "" {
		go func() {
			if err := sendEmail(notifyTo, "Yat Dice: yeni oyuncu şikâyeti", "Moderasyon panelinde incelenmeyi bekleyen yeni bir şikâyet var."); err != nil {
				log.Println("Şikâyet bildirimi gönderilemedi:", err)
			}
		}()
	}

	writeJSON(w, map[string]string{"message": L(r, "Bildiriminiz moderasyon ekibine iletildi.")})
}

// ─── KULLANICI DOĞRULAMA VE HESAP KAPATMA ─────────────────────────────────────

func requireUser(w http.ResponseWriter, r *http.Request) (int, bool) {
	uidStr, err := getUserIDFromToken(r)
	if err != nil {
		httpError(w, r, "Yetkisiz", http.StatusUnauthorized)
		return 0, false
	}
	uid, err := strconv.Atoi(uidStr)
	if err != nil || uid <= 0 {
		httpError(w, r, "Yetkisiz", http.StatusUnauthorized)
		return 0, false
	}
	return uid, true
}

// Token imzası geçerli olsa bile silinmiş, kapatılmış veya şifresi değişmiş hesabı reddeder
func ensureSessionActive(userID int, issuedAt time.Time) error {
	var banned bool
	var validAfter sql.NullTime
	err := db.QueryRow(`SELECT COALESCE(is_banned, FALSE), tokens_valid_after FROM users WHERE id = $1`, userID).Scan(&banned, &validAfter)
	if err != nil {
		return fmt.Errorf("hesap bulunamadı")
	}
	if banned {
		return fmt.Errorf("hesap kapatılmış")
	}
	if validAfter.Valid && issuedAt.Before(validAfter.Time) {
		return fmt.Errorf("oturum geçersiz kılınmış")
	}
	return nil
}

const bannedAccountMessage = "Bu hesap topluluk kuralları ihlali nedeniyle kapatılmıştır."

func isUserBanned(userID int) bool {
	var banned bool
	err := db.QueryRow(`SELECT COALESCE(is_banned, FALSE) FROM users WHERE id = $1`, userID).Scan(&banned)
	return err == nil && banned
}

func banUser(userID int, reason string) error {
	if _, err := db.Exec(`UPDATE users SET is_banned = TRUE, banned_reason = $1 WHERE id = $2`, reason, userID); err != nil {
		return err
	}
	disconnectUser(userID)
	return nil
}

// ─── HESAP VE VERİ SİLME ──────────────────────────────────────────────────────

// Hesabı tek işlemde siler. Çözülmemiş şikâyetler güvenlik amacıyla çözülene kadar saklanır.
func deleteUserData(userID int) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	var appleRefreshToken string
	if err := tx.QueryRow(`SELECT COALESCE(apple_refresh_token, '') FROM users WHERE id = $1`, userID).Scan(&appleRefreshToken); err != nil {
		return err
	}
	if _, err := tx.Exec(`UPDATE user_reports SET reporter_id = 0 WHERE reporter_id = $1`, userID); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM user_reports WHERE reported_user_id = $1 AND status <> 'open'`, userID); err != nil {
		return err
	}
	res, err := tx.Exec(`DELETE FROM users WHERE id = $1`, userID)
	if err != nil {
		return err
	}
	if rows, _ := res.RowsAffected(); rows == 0 {
		return sql.ErrNoRows
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	go func() {
		if err := revokeAppleToken(appleRefreshToken); err != nil {
			log.Println("Apple yetkisi geri alınamadı:", err)
		}
	}()

	mu.Lock()
	delete(pendingInvites, userID)
	mu.Unlock()
	presenceMu.Lock()
	delete(userLastSeen, userID)
	delete(userInGame, userID)
	presenceMu.Unlock()
	resetMu.Lock()
	delete(resetCodes, userID)
	resetMu.Unlock()
	chatRecordsMu.Lock()
	delete(chatRecords, userID)
	chatRecordsMu.Unlock()
	disconnectUser(userID)
	return nil
}

// POST veya DELETE /profile/delete : Kullanıcının hesabını kalıcı olarak siler
func handleDeleteAccount(w http.ResponseWriter, r *http.Request) {
	if setupCORS(&w, r) {
		return
	}
	if !applySecurityAndRateLimit(w, r, true) {
		return
	}
	if r.Method != http.MethodPost && r.Method != http.MethodDelete {
		httpError(w, r, "Geçersiz istek", http.StatusMethodNotAllowed)
		return
	}
	uid, ok := requireUser(w, r)
	if !ok {
		return
	}
	if err := deleteUserData(uid); err != nil {
		httpError(w, r, "Hesap silinirken sunucu hatası oluştu", http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]string{"message": L(r, "Hesabınız kalıcı olarak silindi.")})
}

// GET ve POST /account/delete : Uygulamayı kaldırmış kullanıcılar için web'den silme sayfası
func handleWebAccountDeletion(w http.ResponseWriter, r *http.Request) {
	if !applySecurityAndRateLimit(w, r, true) {
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	if r.Method == http.MethodGet {
		writeLegalPage(w, r, "Hesap Silme", byLang(r, accountDeletionFormHTML, accountDeletionFormHTMLEn))
		return
	}
	if r.Method != http.MethodPost {
		httpError(w, r, "Geçersiz istek", http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseForm(); err != nil {
		httpError(w, r, "Geçersiz istek", http.StatusBadRequest)
		return
	}
	username := strings.TrimSpace(r.PostForm.Get("username"))
	email := strings.ToLower(strings.TrimSpace(r.PostForm.Get("email")))
	password := r.PostForm.Get("password")
	if username == "" || len(username) > 60 || len(email) > 255 || len(password) > 72 || r.PostForm.Get("confirm") != "yes" {
		writeLegalPage(w, r, "Hesap Silme", L(r, `<p>Kullanıcı adı ve onay kutusu zorunludur.</p><p><a href="/account/delete">Geri dön</a></p>`))
		return
	}

	if password != "" {
		if locked, _ := checkLoginLocked(username); !locked {
			var storedHash string
			userID, _, err := resolveHandle(db, username)
			if err == nil {
				err = db.QueryRow(`SELECT password_hash FROM users WHERE id = $1`, userID).Scan(&storedHash)
			}
			if err == nil && bcrypt.CompareHashAndPassword([]byte(storedHash), []byte(password)) == nil {
				if err := deleteUserData(userID); err != nil {
					httpError(w, r, "Hesap silinirken sunucu hatası oluştu", http.StatusInternalServerError)
					return
				}
				resetLoginFailures(username)
				writeLegalPage(w, r, "Hesap Silindi", L(r, `<p>Hesabınız ve ilişkili verileriniz silindi.</p>`))
				return
			}
			recordLoginFailure(username)
		}
	}

	// Şifresiz talepte hesabın varlığı açıklanmaz; talep moderasyon panelinden incelenir
	if _, err := db.Exec(`INSERT INTO deletion_requests (username, email) VALUES ($1, $2)`, username, email); err != nil {
		httpError(w, r, "Talep kaydedilemedi", http.StatusInternalServerError)
		return
	}
	writeLegalPage(w, r, "Talebiniz Alındı", L(r, `<p>Bilgileriniz hesapla eşleşiyorsa talebiniz en geç 30 gün içinde işlenecektir.</p>`))
}

const accountDeletionFormHTML = `<p>Bu sayfadan Yat Dice hesabınızı ve ilişkili verilerinizi silebilirsiniz. Silinen hesap geri getirilemez.</p>
<p><strong>Silinenler:</strong> kullanıcı adı, e-posta, şifre özeti, cihaz kimliği, profil fotoğrafı, ülke, istatistikler, jetonlar, koleksiyon, arkadaşlıklar, arkadaş mesajları ve engeller.</p>
<p><strong>Saklananlar:</strong> Hakkınızda açılmış ve henüz çözülmemiş şikâyet kayıtları güvenlik amacıyla çözülene kadar, çözülen şikâyetler en fazla 90 gün saklanır.</p>
<form method="post" action="/account/delete">
<label>Kullanıcı adı<input name="username" required maxlength="60" autocomplete="username"></label>
<label>Şifre (şifreyle kayıt olduysanız hesap hemen silinir)<input name="password" type="password" maxlength="72" autocomplete="current-password"></label>
<label>Hesap e-postası (Google ile girişte talep doğrulaması için)<input name="email" type="email" maxlength="255" autocomplete="email"></label>
<label class="check"><input name="confirm" type="checkbox" value="yes" required> Hesabımın kalıcı olarak silineceğini anlıyorum.</label>
<button type="submit">Hesabımı Sil</button>
</form>
<p>Misafir hesaplar yalnızca cihazla ilişkilidir; uygulamada <strong>Profili Özelleştir → Hesabımı ve Verilerimi Kalıcı Olarak Sil</strong> adımını kullanın.</p>`

// ─── E-POSTA ──────────────────────────────────────────────────────────────────

func emailConfigured() bool {
	return os.Getenv("SMTP_HOST") != "" && os.Getenv("SMTP_FROM") != ""
}

func validEmailAddress(address string) bool {
	parsed, err := mail.ParseAddress(address)
	return err == nil && parsed.Address == address && len(address) <= 255
}

func buildEmail(from, to, subject, body string) []byte {
	headers := []string{
		"From: " + from,
		"To: " + to,
		"Subject: " + mime.QEncoding.Encode("utf-8", subject),
		"MIME-Version: 1.0",
		"Content-Type: text/plain; charset=UTF-8",
		"Content-Transfer-Encoding: 8bit",
		"Date: " + time.Now().Format(time.RFC1123Z),
	}
	return []byte(strings.Join(headers, "\r\n") + "\r\n\r\n" + strings.ReplaceAll(body, "\n", "\r\n") + "\r\n")
}

func sendEmail(to, subject, body string) error {
	if !emailConfigured() {
		return fmt.Errorf("SMTP yapılandırılmamış")
	}
	if strings.ContainsAny(to, "\r\n") || !validEmailAddress(to) {
		return fmt.Errorf("geçersiz alıcı")
	}
	host := os.Getenv("SMTP_HOST")
	addr := net.JoinHostPort(host, getEnv("SMTP_PORT", "587"))
	from := os.Getenv("SMTP_FROM")

	var auth smtp.Auth
	if user := os.Getenv("SMTP_USER"); user != "" {
		auth = smtp.PlainAuth("", user, os.Getenv("SMTP_PASSWORD"), host)
	}
	return smtp.SendMail(addr, auth, from, []string{to}, buildEmail(from, to, subject, body))
}

// ─── MODERASYON PANELİ ────────────────────────────────────────────────────────

func adminEnabled() bool {
	return len(os.Getenv("ADMIN_TOKEN")) >= minAdminTokenLength
}

func adminAuthorized(r *http.Request) bool {
	if !adminEnabled() {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(r.Header.Get("X-Admin-Token")), []byte(os.Getenv("ADMIN_TOKEN"))) == 1
}

// GET /admin : ADMIN_TOKEN tanımlıysa moderasyon arayüzü
func handleAdminPage(w http.ResponseWriter, r *http.Request) {
	if !applySecurityAndRateLimit(w, r, true) {
		return
	}
	if !adminEnabled() {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; script-src 'unsafe-inline'; connect-src 'self'; img-src data:")
	_, _ = w.Write([]byte(adminPageHTML))
}

// GET /admin/queue : Açık şikâyetleri ve silme taleplerini döner
func handleAdminQueue(w http.ResponseWriter, r *http.Request) {
	if !applySecurityAndRateLimit(w, r, true) {
		return
	}
	if !adminAuthorized(r) {
		httpError(w, r, "Yetkisiz", http.StatusUnauthorized)
		return
	}

	type reportItem struct {
		ID        int             `json:"id"`
		Username  string          `json:"username"`
		Reason    string          `json:"reason"`
		Evidence  json.RawMessage `json:"evidence"`
		Avatar    string          `json:"avatar"`
		CreatedAt time.Time       `json:"createdAt"`
	}
	type deletionItem struct {
		ID         int       `json:"id"`
		Username   string    `json:"username"`
		Email      string    `json:"email"`
		EmailMatch bool      `json:"emailMatch"`
		CreatedAt  time.Time `json:"createdAt"`
	}

	reports := []reportItem{}
	rows, err := db.Query(`
		SELECT r.id, r.reported_username, r.reason, COALESCE(NULLIF(r.evidence, ''), '[]'), COALESCE(p.avatar, ''), r.created_at
		FROM user_reports r LEFT JOIN profiles p ON p.user_id = r.reported_user_id
		WHERE r.status = 'open' ORDER BY r.id LIMIT 100
	`)
	if err != nil {
		httpError(w, r, "Kuyruk alınamadı", http.StatusInternalServerError)
		return
	}
	defer rows.Close()
	for rows.Next() {
		var item reportItem
		var evidence string
		if rows.Scan(&item.ID, &item.Username, &item.Reason, &evidence, &item.Avatar, &item.CreatedAt) == nil {
			item.Evidence = json.RawMessage(evidence)
			reports = append(reports, item)
		}
	}

	deletions := []deletionItem{}
	deletionRows, err := db.Query(`
		SELECT d.id, d.username, d.email, d.created_at,
		       EXISTS(SELECT 1 FROM users u WHERE (LOWER(u.username) = LOWER(d.username) OR LOWER(split_part(u.username, '#', 1)) = LOWER(d.username))
		              AND d.email <> '' AND LOWER(u.email) = LOWER(d.email))
		FROM deletion_requests d WHERE d.status = 'open' ORDER BY d.id LIMIT 100
	`)
	if err != nil {
		httpError(w, r, "Kuyruk alınamadı", http.StatusInternalServerError)
		return
	}
	defer deletionRows.Close()
	for deletionRows.Next() {
		var item deletionItem
		if deletionRows.Scan(&item.ID, &item.Username, &item.Email, &item.CreatedAt, &item.EmailMatch) == nil {
			deletions = append(deletions, item)
		}
	}

	writeJSON(w, map[string]interface{}{"reports": reports, "deletions": deletions})
}

// POST /admin/action : Şikâyeti veya silme talebini sonuçlandırır
func handleAdminAction(w http.ResponseWriter, r *http.Request) {
	if !applySecurityAndRateLimit(w, r, true) {
		return
	}
	if !adminAuthorized(r) {
		httpError(w, r, "Yetkisiz", http.StatusUnauthorized)
		return
	}
	var req struct {
		Kind   string `json:"kind"`
		ID     int    `json:"id"`
		Action string `json:"action"`
	}
	if r.Method != http.MethodPost || json.NewDecoder(r.Body).Decode(&req) != nil {
		httpError(w, r, "Geçersiz istek", http.StatusBadRequest)
		return
	}

	var err error
	switch {
	case req.Kind == "report" && req.Action == "ban":
		var targetID sql.NullInt64
		var reason string
		err = db.QueryRow(`SELECT reported_user_id, reason FROM user_reports WHERE id = $1 AND status = 'open'`, req.ID).Scan(&targetID, &reason)
		if err == nil && targetID.Valid {
			err = banUser(int(targetID.Int64), reason)
		}
		if err == nil {
			_, err = db.Exec(`UPDATE user_reports SET status = 'banned', resolved_at = NOW() WHERE id = $1 OR (status = 'open' AND reported_user_id = $2)`, req.ID, targetID)
		}
	case req.Kind == "report" && req.Action == "clear_avatar":
		// Uygunsuz profil fotoğrafını kaldırır; hesap açık kalır
		var targetID sql.NullInt64
		err = db.QueryRow(`SELECT reported_user_id FROM user_reports WHERE id = $1 AND status = 'open'`, req.ID).Scan(&targetID)
		if err == nil && targetID.Valid {
			_, err = db.Exec(`UPDATE profiles SET avatar = '' WHERE user_id = $1`, targetID.Int64)
		}
		if err == nil {
			_, err = db.Exec(`UPDATE user_reports SET status = 'content_removed', resolved_at = NOW() WHERE id = $1`, req.ID)
		}
	case req.Kind == "report" && req.Action == "dismiss":
		_, err = db.Exec(`UPDATE user_reports SET status = 'dismissed', resolved_at = NOW() WHERE id = $1`, req.ID)
	case req.Kind == "deletion" && req.Action == "delete":
		var username string
		err = db.QueryRow(`SELECT username FROM deletion_requests WHERE id = $1 AND status = 'open'`, req.ID).Scan(&username)
		if err == nil {
			if userID, _, findErr := userIDByName(username); findErr == nil {
				err = deleteUserData(userID)
			}
		}
		if err == nil {
			_, err = db.Exec(`DELETE FROM deletion_requests WHERE id = $1`, req.ID)
		}
	case req.Kind == "deletion" && req.Action == "dismiss":
		_, err = db.Exec(`DELETE FROM deletion_requests WHERE id = $1`, req.ID)
	default:
		httpError(w, r, "Geçersiz işlem", http.StatusBadRequest)
		return
	}
	if err != nil {
		httpError(w, r, "İşlem tamamlanamadı", http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]string{"status": "ok"})
}

func pruneResolvedReports() {
	if _, err := db.Exec(`DELETE FROM user_reports WHERE status <> 'open' AND resolved_at < NOW() - INTERVAL '90 days'`); err != nil {
		log.Println("Eski şikâyetler temizlenemedi:", err)
	}
}

func writeJSON(w http.ResponseWriter, payload interface{}) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(payload)
}

const adminPageHTML = `<!doctype html><html lang="tr"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>Yat Dice Moderasyon</title><style>
body{font-family:system-ui,sans-serif;background:#111318;color:#e5e7eb;max-width:760px;margin:0 auto;padding:20px}h1,h2{color:#4aada6}
.card{background:#1f232b;border:1px solid #333845;border-radius:12px;padding:14px;margin:10px 0}.muted{color:#8b909b;font-size:13px}
input{width:100%;box-sizing:border-box;padding:10px;border-radius:8px;border:1px solid #333845;background:#111318;color:#e5e7eb;margin:8px 0}
button{padding:8px 12px;border:0;border-radius:8px;margin:6px 6px 0 0;cursor:pointer;font-weight:700;background:#4aada6;color:#111318}button.danger{background:#c0392b;color:#fff}li{margin:3px 0}
</style></head><body><h1>Yat Dice Moderasyon</h1><input id="token" type="password" placeholder="ADMIN_TOKEN" autocomplete="off"><button id="load">Kuyruğu Yükle</button><p id="status" class="muted"></p><h2>Şikâyetler</h2><div id="reports"></div><h2>Silme Talepleri</h2><div id="deletions"></div>
<script>
const el=(tag,text,cls)=>{const e=document.createElement(tag);if(text!==undefined)e.textContent=text;if(cls)e.className=cls;return e};
const token=()=>document.getElementById('token').value;
const status=t=>document.getElementById('status').textContent=t;
async function act(kind,id,action,label){if(!confirm(label+'?'))return;const r=await fetch('/admin/action',{method:'POST',headers:{'Content-Type':'application/json','X-Admin-Token':token()},body:JSON.stringify({kind,id,action})});status(r.ok?'Tamamlandı':'Hata: '+r.status);load()}
function button(text,cls,fn){const b=el('button',text,cls);b.onclick=fn;return b}
async function load(){const r=await fetch('/admin/queue',{headers:{'X-Admin-Token':token()}});if(!r.ok){status('Yetkisiz veya hata: '+r.status);return}const d=await r.json();status(d.reports.length+' şikâyet, '+d.deletions.length+' silme talebi');
const rs=document.getElementById('reports');rs.replaceChildren();d.reports.forEach(x=>{const c=el('div',undefined,'card');c.append(el('strong',x.username),el('p',x.reason));if((x.avatar||'').startsWith('data:image/')){const i=el('img');i.src=x.avatar;i.alt='Profil fotoğrafı';i.style.cssText='width:96px;height:96px;object-fit:cover;border-radius:12px;display:block';c.append(i,button('Fotoğrafı Kaldır','',()=>act('report',x.id,'clear_avatar','Profil fotoğrafı kaldırılsın')))}c.append(el('p',new Date(x.createdAt).toLocaleString('tr-TR'),'muted'));const l=el('ul');(x.evidence||[]).forEach(m=>l.append(el('li',new Date(m.at).toLocaleTimeString('tr-TR')+' — '+m.text)));if(!(x.evidence||[]).length)c.append(el('p','Sohbet kaydı yok','muted'));c.append(l,button('Hesabı Kapat','danger',()=>act('report',x.id,'ban','Hesap kapatılsın')),button('Reddet','',()=>act('report',x.id,'dismiss','Şikâyet reddedilsin')));rs.append(c)});
const ds=document.getElementById('deletions');ds.replaceChildren();d.deletions.forEach(x=>{const c=el('div',undefined,'card');c.append(el('strong',x.username),el('p',(x.email||'E-posta yok')+(x.emailMatch?' — hesapla eşleşiyor':' — eşleşmiyor'),'muted'),button('Hesabı Sil','danger',()=>act('deletion',x.id,'delete','Hesap silinsin')),button('Reddet','',()=>act('deletion',x.id,'dismiss','Talep reddedilsin')));ds.append(c)})}
document.getElementById('load').onclick=load;
</script></body></html>`

// ─── GİZLİLİK POLİTİKASI VE KULLANIM KOŞULLARI ────────────────────────────────

func legalContactHTML(r *http.Request) string {
	owner := strings.TrimSpace(os.Getenv("LEGAL_OWNER_NAME"))
	contact := strings.TrimSpace(os.Getenv("LEGAL_CONTACT_EMAIL"))
	if owner == "" {
		owner = L(r, "Yat Dice geliştiricisi")
	}
	if contact == "" {
		return owner
	}
	return owner + " — <a href=\"mailto:" + contact + "\">" + contact + "</a>"
}

func writeLegalPage(w http.ResponseWriter, r *http.Request, title, body string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	title = L(r, title)
	page := `<!doctype html><html lang="` + requestLang(r) + `"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>Yat Dice — ` + title + `</title><style>body{font-family:system-ui,-apple-system,sans-serif;background:#111318;color:#e5e7eb;max-width:740px;margin:0 auto;padding:32px 20px;line-height:1.65}h1,h2{color:#4aada6}a{color:#c9a84c}label{display:block;margin:14px 0 4px}label.check{display:flex;gap:8px;align-items:flex-start}input:not([type=checkbox]){display:block;width:100%;box-sizing:border-box;margin-top:6px;padding:11px;border-radius:8px;border:1px solid #333845;background:#1f232b;color:#e5e7eb}button{margin-top:14px;padding:12px 18px;border:0;border-radius:8px;background:#c0392b;color:#fff;font-weight:700;cursor:pointer}</style></head><body><h1>Yat Dice — ` + title + `</h1>` + body + `</body></html>`
	_, _ = w.Write([]byte(page))
}

// GET /privacy
func handlePrivacyHTML(w http.ResponseWriter, r *http.Request) {
	writeLegalPage(w, r, "Gizlilik Politikası", strings.ReplaceAll(byLang(r, privacyPolicyHTML, privacyPolicyHTMLEn), "{{CONTACT}}", legalContactHTML(r)))
}

// GET /terms
func handleTermsHTML(w http.ResponseWriter, r *http.Request) {
	writeLegalPage(w, r, "Kullanım Koşulları", strings.ReplaceAll(byLang(r, termsOfUseHTML, termsOfUseHTMLEn), "{{CONTACT}}", legalContactHTML(r)))
}

// GET /support : Mağazalarda destek adresi olarak da kullanılır
func handleSupportHTML(w http.ResponseWriter, r *http.Request) {
	writeLegalPage(w, r, "Destek", strings.ReplaceAll(byLang(r, supportHTML, supportHTMLEn), "{{CONTACT}}", legalContactHTML(r)))
}

// byLang isteğin diline göre Türkçe veya İngilizce sayfa metnini seçer.
func byLang(r *http.Request, tr, en string) string {
	if requestLang(r) == langEN {
		return en
	}
	return tr
}

const supportHTML = `<p>Soru, hata bildirimi, şikâyet veya veri talepleriniz için bize e-postayla ulaşabilirsiniz:</p>
<p>{{CONTACT}}</p>
<h2>Sık Sorulanlar</h2>
<p><strong>Bir oyuncuyu nasıl şikâyet ederim?</strong> Maç sırasında sohbet menüsünden veya liderlik tablosundaki bayrak simgesinden şikâyet edebilir, oyuncuyu engelleyebilirsiniz.</p>
<p><strong>Hesabımı nasıl silerim?</strong> Uygulamada <strong>Profili Özelleştir → Hesabımı ve Verilerimi Kalıcı Olarak Sil</strong> adımını veya <a href="/account/delete">web'den hesap silme sayfasını</a> kullanın.</p>
<p><strong>Jetonlar gerçek parayla alınabilir mi?</strong> Hayır. Jeton ve kasalar yalnızca oyun içi sanal öğelerdir; satın alınamaz ve paraya çevrilemez.</p>
<p><a href="/privacy">Gizlilik Politikası</a> · <a href="/terms">Kullanım Koşulları</a></p>`

const privacyPolicyHTML = `<p><strong>Son güncelleme:</strong> Kasım 2026</p>
<h2>1. Veri Sorumlusu</h2><p>{{CONTACT}}</p>
<h2>2. İşlenen Veriler</h2><ul>
<li><strong>Hesap:</strong> kullanıcı adı, şifre özeti, varsa e-posta adresi, Google veya Apple hesap kimliği.</li>
<li><strong>Misafir hesap:</strong> uygulamanın ürettiği rastgele cihaz kimliği. Reklam kimliği kullanılmaz.</li>
<li><strong>Profil ve oyun:</strong> profil fotoğrafı veya avatar, ülke tercihi, XP, jeton, kasa, istatistikler, görevler ve zar koleksiyonu.</li>
<li><strong>Sosyal veriler:</strong> arkadaşlıklar, oyun davetleri, engeller ve şikâyet kayıtları.</li>
<li><strong>Maç sohbeti:</strong> mesajlar kalıcı olarak kaydedilmez. Şikâyet incelemesi için son mesajlar en fazla 30 dakika bellekte tutulur; şikâyet edilirse şikâyet edilen oyuncunun bu mesajları şikâyet kaydına eklenir.</li>
<li><strong>Arkadaş mesajları:</strong> yalnızca arkadaşlar arasında gönderilebilir; teslim edilmek için en fazla 30 gün saklanır ve hesap silinince silinir. Şikâyet edilirse şikâyet edilen oyuncunun son 7 günde size yazdığı mesajlar şikâyet kaydına eklenir.</li>
<li><strong>Teknik veriler:</strong> IP adresi güvenlik ve hız sınırlaması için işlenir; barındırma sağlayıcısının sistem kayıtlarında geçici olarak bulunabilir.</li></ul>
<h2>3. Toplama Yöntemi, Amaçlar ve Hukuki Sebepler</h2><p>Veriler, uygulama ve web sitesi üzerinden elektronik ortamda; sizin girdiğiniz bilgilerle, seçtiğiniz giriş sağlayıcısından (Google veya Apple) gelen bilgilerle ve oyunu oynarken otomatik yollarla toplanır. Veriler hesabı ve oyunu sunmak, eşleştirme ve sosyal özellikleri çalıştırmak, şifre kurtarmak, kötüye kullanımı önlemek ve yasal yükümlülükleri yerine getirmek için işlenir. Hukuki sebepler sözleşmenin ifası, meşru menfaat ve gerektiğinde açık rızadır.</p>
<h2>4. Paylaşım</h2><p>Veriler satılmaz ve reklam amacıyla paylaşılmaz. Hizmet için gerekli ölçüde barındırma ve veritabanı sağlayıcıları, e-posta gönderim sağlayıcısı ve kullandığınız giriş yöntemine göre Google veya Apple ile paylaşılabilir. Bu sağlayıcılar yurt dışında bulunabilir. Kullanıcı adı, avatar, ülke ve oyun istatistikleri diğer oyunculara görünür.</p>
<h2>5. Saklama Süreleri</h2><p>Hesap verileri hesap silinene kadar saklanır. Çözülmemiş şikâyetler çözülene kadar, çözülen şikâyetler en fazla 90 gün saklanır. Kapatılan hesapların kötüye kullanımın tekrarını önlemek için gerekli kayıtları saklanabilir.</p>
<h2>6. Haklarınız</h2><p>KVKK ve GDPR kapsamında verilerinize erişme, düzeltme, silme, işlemeyi kısıtlama, itiraz etme ve veri taşınabilirliği haklarına sahip olabilirsiniz. Başvurularınızı aşağıdaki iletişim adresine iletebilir, yetkili veri koruma kurumuna şikâyette bulunabilirsiniz.</p>
<h2>7. Hesap Silme</h2><p>Uygulamada <strong>Profili Özelleştir → Hesabımı ve Verilerimi Kalıcı Olarak Sil</strong> adımını veya <a href="/account/delete">web'den hesap silme sayfasını</a> kullanabilirsiniz.</p>
<h2>8. Çocuklar</h2><p>Yat Dice 13 yaşın altındaki çocuklara yönelik değildir ve bu yaş grubundan bilerek veri toplamaz.</p>
<h2>9. Güvenlik ve Değişiklikler</h2><p>Şifreler özetlenerek saklanır ve bağlantılar üretimde şifrelenir. Bu politika güncellenirse yeni sürüm bu sayfada yayımlanır.</p>
<h2>10. İletişim</h2><p>{{CONTACT}}</p>`

const termsOfUseHTML = `<p><strong>Son güncelleme:</strong> Kasım 2026</p>
<h2>1. Kabul ve Yaş Şartı</h2><p>Yat Dice'ı kullanarak bu koşulları ve <a href="/privacy">Gizlilik Politikası</a>'nı kabul edersiniz. Hizmeti kullanmak için en az 13 yaşında olmalısınız.</p>
<h2>2. Lisans</h2><p>Size oyunu kişisel ve ticari olmayan amaçla kullanmanız için kişisel, devredilemez ve geri alınabilir bir lisans verilir. Oyunu kopyalayamaz, tersine mühendislik yapamaz veya hile amacıyla değiştiremezsiniz.</p>
<h2>3. Hesabınız</h2><p>Hesabınızın güvenliğinden siz sorumlusunuz. Başkasının kimliğine bürünmek veya başkasının hesabını kullanmak yasaktır.</p>
<h2>4. Topluluk Kuralları: Sıfır Tolerans</h2><p>Hakaret, küfür, nefret söylemi, taciz, tehdit, cinsel içerik, spam, uygunsuz profil fotoğrafı veya kullanıcı adı ve hile kesinlikle yasaktır. Kullanıcılar oyuncuları şikâyet edebilir ve engelleyebilir. Şikâyetler incelenir; ihlal eden içerik kaldırılır ve hesaplar kalıcı olarak kapatılabilir.</p>
<h2>5. Sanal Öğeler</h2><p>Jetonlar, kasalar ve zarlar yalnızca oyun içi sanal öğelerdir. Gerçek parayla satın alınamaz, gerçek paraya veya değere dönüştürülemez ve devredilemez. Hesap silindiğinde veya kapatıldığında kaybolur.</p>
<h2>6. Kasa Olasılıkları</h2><p>Her kasa 50 ile 199 arasında sanal jeton verir ve her değerin olasılığı eşittir: 50–99 %33,3, 100–149 %33,3, 150–199 %33,4.</p>
<h2>7. Hizmetin Sunulması</h2><p>Hizmet olduğu gibi sunulur; kesintisiz çalışma garanti edilmez. Bağlantı sorunlarında 60 saniye içinde maça dönülmezse maç hükmen kaybedilebilir. Uygulanabilir hukukun izin verdiği ölçüde dolaylı zararlardan sorumluluk kabul edilmez; tüketici olarak sahip olduğunuz zorunlu haklar saklıdır.</p>
<h2>8. Uygulama Mağazaları</h2><p>Bu koşullar sizinle geliştirici arasındadır. Apple ve Google hizmetten, içeriğinden, bakımından veya desteğinden sorumlu değildir; ancak Apple ve bağlı şirketleri, App Store üzerinden indirilen uygulamalar bakımından bu koşulların üçüncü taraf lehtarıdır.</p>
<h2>9. Fesih ve Değişiklikler</h2><p>Hesabınızı istediğiniz zaman silebilirsiniz. Koşulları ihlal eden hesaplar kapatılabilir. Koşullar güncellenirse yeni sürüm bu sayfada yayımlanır.</p>
<h2>10. İletişim</h2><p>{{CONTACT}}</p>`
