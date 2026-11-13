package main

import (
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"math/big"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

const appleIssuer = "https://appleid.apple.com"

var (
	appleKeysURL   = appleIssuer + "/auth/keys"
	appleTokenURL  = appleIssuer + "/auth/token"
	appleRevokeURL = appleIssuer + "/auth/revoke"

	appleKeysMu        sync.Mutex
	appleKeys          = make(map[string]*rsa.PublicKey)
	appleKeysFetchedAt time.Time
)

type appleIdentity struct {
	Sub      string
	Email    string
	Audience string
}

func parseAppleKeys(body []byte) (map[string]*rsa.PublicKey, error) {
	var set struct {
		Keys []struct {
			Kid string `json:"kid"`
			Kty string `json:"kty"`
			N   string `json:"n"`
			E   string `json:"e"`
		} `json:"keys"`
	}
	if err := json.Unmarshal(body, &set); err != nil {
		return nil, err
	}
	keys := make(map[string]*rsa.PublicKey)
	for _, key := range set.Keys {
		if key.Kty != "RSA" {
			continue
		}
		modulus, err := base64.RawURLEncoding.DecodeString(key.N)
		if err != nil {
			continue
		}
		exponent, err := base64.RawURLEncoding.DecodeString(key.E)
		if err != nil {
			continue
		}
		keys[key.Kid] = &rsa.PublicKey{N: new(big.Int).SetBytes(modulus), E: int(new(big.Int).SetBytes(exponent).Int64())}
	}
	return keys, nil
}

func appleKey(kid string) (*rsa.PublicKey, error) {
	appleKeysMu.Lock()
	defer appleKeysMu.Unlock()
	if key, ok := appleKeys[kid]; ok && time.Since(appleKeysFetchedAt) < 24*time.Hour {
		return key, nil
	}
	if time.Since(appleKeysFetchedAt) < time.Minute {
		return nil, fmt.Errorf("bilinmeyen Apple anahtarı")
	}

	resp, err := (&http.Client{Timeout: 5 * time.Second}).Get(appleKeysURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Apple anahtarları alınamadı: %d", resp.StatusCode)
	}
	var raw json.RawMessage
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, err
	}
	keys, err := parseAppleKeys(raw)
	if err != nil {
		return nil, err
	}
	appleKeys = keys
	appleKeysFetchedAt = time.Now()
	if key, ok := keys[kid]; ok {
		return key, nil
	}
	return nil, fmt.Errorf("bilinmeyen Apple anahtarı")
}

// Apple kimlik token'ını imza, yayıncı, süre, uygulama kimliği ve nonce ile doğrular
func verifyAppleIDToken(tokenString string, audiences []string, nonce string) (appleIdentity, error) {
	claims := jwt.MapClaims{}
	_, err := jwt.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (interface{}, error) {
		kid, _ := token.Header["kid"].(string)
		return appleKey(kid)
	}, jwt.WithValidMethods([]string{"RS256"}), jwt.WithIssuer(appleIssuer), jwt.WithExpirationRequired())
	if err != nil {
		return appleIdentity{}, err
	}

	tokenAudiences, err := claims.GetAudience()
	if err != nil {
		return appleIdentity{}, err
	}
	matched := ""
	for _, allowed := range audiences {
		for _, audience := range tokenAudiences {
			if audience == allowed {
				matched = allowed
			}
		}
	}
	if matched == "" {
		return appleIdentity{}, fmt.Errorf("beklenmeyen uygulama kimliği")
	}
	if tokenNonce, _ := claims["nonce"].(string); nonce == "" || tokenNonce != nonce {
		return appleIdentity{}, fmt.Errorf("nonce eşleşmedi")
	}
	sub, _ := claims["sub"].(string)
	if sub == "" {
		return appleIdentity{}, fmt.Errorf("Apple kimliği yok")
	}
	email := ""
	if fmt.Sprint(claims["email_verified"]) == "true" {
		address, _ := claims["email"].(string)
		email = strings.ToLower(strings.TrimSpace(address))
	}
	return appleIdentity{Sub: sub, Email: email, Audience: matched}, nil
}

// Google ve Apple ile ilk girişte hesap ve profil oluşturur. providerColumn yalnızca sabit değerler alır.
func createProviderUser(providerColumn, providerID, email, displayName string) (int, string, error) {
	baseName := displayName
	if baseName == "" && email != "" {
		baseName = strings.Split(email, "@")[0]
	}
	baseName = strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' {
			return r
		}
		return -1
	}, baseName)
	if runes := []rune(baseName); len(runes) > 24 {
		baseName = string(runes[:24])
	}
	if len([]rune(baseName)) < 2 || isForbiddenUsername(baseName) {
		baseName = "Oyuncu"
	}

	query := fmt.Sprintf(`
		INSERT INTO users (username, password_hash, email, %s, is_guest, terms_accepted_at)
		VALUES ($1, $2, $3, $4, FALSE, NOW())
		RETURNING id, username
	`, providerColumn)

	randomPass, err := bcrypt.GenerateFromPassword([]byte(fmt.Sprintf("%s-%s-%d", providerColumn, providerID, time.Now().UnixNano())), BcryptCost)
	if err != nil {
		return 0, "", err
	}
	userID, username, err := insertWithHandle(baseName, func(candidate string) (int, error) {
		var id int
		var stored string
		err := db.QueryRow(query, candidate, string(randomPass), email, providerID).Scan(&id, &stored)
		return id, err
	})
	if err != nil {
		return 0, "", err
	}
	if _, err := db.Exec(`
		INSERT INTO profiles (user_id, xp, tokens, cases, unlocked_dice, selected_dice)
		VALUES ($1, 150, 300, 2, 'ivory', 'ivory')
		ON CONFLICT (user_id) DO NOTHING
	`, userID); err != nil {
		return 0, "", err
	}
	return userID, username, nil
}

// POST /auth/apple : Sign in with Apple ile giriş ve kayıt
func handleAppleAuth(w http.ResponseWriter, r *http.Request) {
	if setupCORS(&w, r) {
		return
	}
	if !applySecurityAndRateLimit(w, r, true) {
		return
	}
	audiences := splitCSV(os.Getenv("APPLE_CLIENT_IDS"))
	if len(audiences) == 0 {
		httpError(w, r, "Apple ile giriş bu sunucuda etkin değil", http.StatusServiceUnavailable)
		return
	}

	var req struct {
		Credential        string `json:"credential"`
		AuthorizationCode string `json:"authorizationCode"`
		Nonce             string `json:"nonce"`
		AcceptedTerms     bool   `json:"acceptedTerms"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Credential == "" {
		httpError(w, r, "Apple kimlik bilgisi gerekli", http.StatusBadRequest)
		return
	}
	identity, err := verifyAppleIDToken(req.Credential, audiences, req.Nonce)
	if err != nil {
		log.Println("Apple token doğrulanamadı:", err)
		httpError(w, r, "Apple oturumu doğrulanamadı", http.StatusUnauthorized)
		return
	}

	var userID int
	var username string
	err = db.QueryRow(`SELECT id, username FROM users WHERE apple_id = $1 LIMIT 1`, identity.Sub).Scan(&userID, &username)
	if err == nil && isUserBanned(userID) {
		httpError(w, r, bannedAccountMessage, http.StatusForbidden)
		return
	}
	if err != nil {
		if !req.AcceptedTerms {
			httpError(w, r, termsRequiredMessage, http.StatusBadRequest)
			return
		}
		if identity.Email != "" {
			var emailTaken bool
			if qErr := db.QueryRow(`SELECT EXISTS(SELECT 1 FROM users WHERE LOWER(email) = LOWER($1))`, identity.Email).Scan(&emailTaken); qErr != nil {
				httpError(w, r, "Sunucu hatası", http.StatusInternalServerError)
				return
			}
			if emailTaken {
				httpError(w, r, "Bu e-posta ile kayıtlı bir hesap var. Lütfen diğer giriş yönteminizi kullanın.", http.StatusConflict)
				return
			}
		}
		userID, username, err = createProviderUser("apple_id", identity.Sub, identity.Email, "")
		if err != nil {
			httpError(w, r, "Apple hesabı oluşturulamadı", http.StatusInternalServerError)
			return
		}
	}

	if req.AuthorizationCode != "" {
		if refreshToken, err := exchangeAppleCode(req.AuthorizationCode, identity.Audience); err != nil {
			log.Println("Apple yetki kodu alınamadı:", err)
		} else if refreshToken != "" {
			_, _ = db.Exec(`UPDATE users SET apple_refresh_token = $1 WHERE id = $2`, refreshToken, userID)
		}
	}

	tokenString, err := issueJWTForUser(userID, 30)
	if err != nil {
		httpError(w, r, "Token oluşturulamadı", http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]interface{}{"token": tokenString, "username": username, "isGuest": false})
}

func appleRevocationConfigured() bool {
	return os.Getenv("APPLE_TEAM_ID") != "" && os.Getenv("APPLE_KEY_ID") != "" && os.Getenv("APPLE_PRIVATE_KEY") != ""
}

func appleClientSecret(clientID string) (string, error) {
	pem := strings.ReplaceAll(os.Getenv("APPLE_PRIVATE_KEY"), `\n`, "\n")
	key, err := jwt.ParseECPrivateKeyFromPEM([]byte(pem))
	if err != nil {
		return "", err
	}
	now := time.Now()
	token := jwt.NewWithClaims(jwt.SigningMethodES256, jwt.RegisteredClaims{
		Issuer:    os.Getenv("APPLE_TEAM_ID"),
		Subject:   clientID,
		Audience:  jwt.ClaimStrings{appleIssuer},
		IssuedAt:  jwt.NewNumericDate(now),
		ExpiresAt: jwt.NewNumericDate(now.Add(5 * time.Minute)),
	})
	token.Header["kid"] = os.Getenv("APPLE_KEY_ID")
	return token.SignedString(key)
}

func exchangeAppleCode(code, clientID string) (string, error) {
	if !appleRevocationConfigured() {
		return "", nil
	}
	secret, err := appleClientSecret(clientID)
	if err != nil {
		return "", err
	}
	resp, err := (&http.Client{Timeout: 8 * time.Second}).PostForm(appleTokenURL, url.Values{
		"client_id":     {clientID},
		"client_secret": {secret},
		"code":          {code},
		"grant_type":    {"authorization_code"},
	})
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("Apple token durumu: %d", resp.StatusCode)
	}
	var body struct {
		RefreshToken string `json:"refresh_token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return "", err
	}
	return body.RefreshToken, nil
}

// Hesap silinince Apple yetkisini geri alır (App Store Guideline 5.1.1(v))
func revokeAppleToken(refreshToken string) error {
	audiences := splitCSV(os.Getenv("APPLE_CLIENT_IDS"))
	if refreshToken == "" || len(audiences) == 0 || !appleRevocationConfigured() {
		return nil
	}
	secret, err := appleClientSecret(audiences[0])
	if err != nil {
		return err
	}
	resp, err := (&http.Client{Timeout: 8 * time.Second}).PostForm(appleRevokeURL, url.Values{
		"client_id":       {audiences[0]},
		"client_secret":   {secret},
		"token":           {refreshToken},
		"token_type_hint": {"refresh_token"},
	})
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("Apple iptal durumu: %d", resp.StatusCode)
	}
	return nil
}
