package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const testAppleAudience = "com.example.app"

func startFakeAppleKeys(t *testing.T, key *rsa.PublicKey) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"keys": []map[string]string{{
			"kid": "test-key",
			"kty": "RSA",
			"n":   base64.RawURLEncoding.EncodeToString(key.N.Bytes()),
			"e":   base64.RawURLEncoding.EncodeToString(big.NewInt(int64(key.E)).Bytes()),
		}}})
	}))
	originalURL := appleKeysURL
	appleKeysURL = srv.URL
	appleKeysMu.Lock()
	appleKeys = make(map[string]*rsa.PublicKey)
	appleKeysFetchedAt = time.Time{}
	appleKeysMu.Unlock()
	t.Cleanup(func() {
		srv.Close()
		appleKeysURL = originalURL
		appleKeysMu.Lock()
		appleKeys = make(map[string]*rsa.PublicKey)
		appleKeysFetchedAt = time.Time{}
		appleKeysMu.Unlock()
	})
}

func signAppleToken(t *testing.T, key *rsa.PrivateKey, kid string, overrides jwt.MapClaims) string {
	t.Helper()
	claims := jwt.MapClaims{
		"iss":            appleIssuer,
		"aud":            testAppleAudience,
		"sub":            "apple-user",
		"exp":            time.Now().Add(time.Hour).Unix(),
		"nonce":          "nonce-1",
		"email":          "A@privaterelay.appleid.com",
		"email_verified": "true",
	}
	for name, value := range overrides {
		claims[name] = value
	}
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	token.Header["kid"] = kid
	signed, err := token.SignedString(key)
	if err != nil {
		t.Fatal(err)
	}
	return signed
}

func TestVerifyAppleIDToken(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	otherKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	startFakeAppleKeys(t, &key.PublicKey)

	cases := []struct {
		name    string
		token   string
		nonce   string
		wantErr bool
	}{
		{"valid", signAppleToken(t, key, "test-key", nil), "nonce-1", false},
		{"forged signature", signAppleToken(t, otherKey, "test-key", nil), "nonce-1", true},
		{"wrong issuer", signAppleToken(t, key, "test-key", jwt.MapClaims{"iss": "https://evil.example"}), "nonce-1", true},
		{"other app", signAppleToken(t, key, "test-key", jwt.MapClaims{"aud": "com.other.app"}), "nonce-1", true},
		{"expired", signAppleToken(t, key, "test-key", jwt.MapClaims{"exp": time.Now().Add(-time.Hour).Unix()}), "nonce-1", true},
		{"wrong nonce", signAppleToken(t, key, "test-key", nil), "other", true},
		{"missing nonce", signAppleToken(t, key, "test-key", nil), "", true},
		{"missing subject", signAppleToken(t, key, "test-key", jwt.MapClaims{"sub": ""}), "nonce-1", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			identity, err := verifyAppleIDToken(tc.token, []string{testAppleAudience}, tc.nonce)
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tc.wantErr)
			}
			if err == nil && (identity.Sub != "apple-user" || identity.Email != "a@privaterelay.appleid.com") {
				t.Fatalf("unexpected identity: %+v", identity)
			}
		})
	}
}

func TestAppleEmailIgnoredUnlessVerified(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	startFakeAppleKeys(t, &key.PublicKey)
	token := signAppleToken(t, key, "test-key", jwt.MapClaims{"email_verified": "false"})
	identity, err := verifyAppleIDToken(token, []string{testAppleAudience}, "nonce-1")
	if err != nil || identity.Email != "" {
		t.Fatalf("identity %+v, err %v", identity, err)
	}
}

func TestAppleTokenRevocation(t *testing.T) {
	ecKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(ecKey)
	if err != nil {
		t.Fatal(err)
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})
	t.Setenv("APPLE_CLIENT_IDS", testAppleAudience)
	t.Setenv("APPLE_TEAM_ID", "TEAM123")
	t.Setenv("APPLE_KEY_ID", "KEY123")
	t.Setenv("APPLE_PRIVATE_KEY", strings.ReplaceAll(string(keyPEM), "\n", `\n`))

	var received map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		received = map[string]string{"token": r.PostForm.Get("token"), "client": r.PostForm.Get("client_id"), "secret": r.PostForm.Get("client_secret")}
	}))
	defer srv.Close()
	original := appleRevokeURL
	appleRevokeURL = srv.URL
	defer func() { appleRevokeURL = original }()

	if err := revokeAppleToken("refresh-1"); err != nil {
		t.Fatal(err)
	}
	if received["token"] != "refresh-1" || received["client"] != testAppleAudience {
		t.Fatalf("unexpected request: %+v", received)
	}
	claims := jwt.RegisteredClaims{}
	if _, err := jwt.ParseWithClaims(received["secret"], &claims, func(*jwt.Token) (interface{}, error) { return &ecKey.PublicKey, nil }, jwt.WithValidMethods([]string{"ES256"})); err != nil {
		t.Fatal(err)
	}
	if claims.Issuer != "TEAM123" || claims.Subject != testAppleAudience {
		t.Fatalf("unexpected client secret claims: %+v", claims)
	}
}

func TestAppleAuthCreatesAndReusesAccount(t *testing.T) {
	openTestDB(t)
	jwtKey = []byte(strings.Repeat("k", minJWTSecretLength))
	t.Setenv("APPLE_CLIENT_IDS", testAppleAudience)
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	startFakeAppleKeys(t, &key.PublicKey)

	sub := fmt.Sprintf("apple-%d", time.Now().UnixNano())
	token := signAppleToken(t, key, "test-key", jwt.MapClaims{"sub": sub, "email": sub + "@privaterelay.appleid.com"})
	call := func(accepted bool) *httptest.ResponseRecorder {
		body, _ := json.Marshal(map[string]interface{}{"credential": token, "nonce": "nonce-1", "acceptedTerms": accepted})
		recorder := httptest.NewRecorder()
		handleAppleAuth(recorder, httptest.NewRequest(http.MethodPost, "/auth/apple", strings.NewReader(string(body))))
		return recorder
	}

	if rec := call(false); rec.Code != http.StatusBadRequest {
		t.Fatalf("new account without terms: got %d", rec.Code)
	}
	first := call(true)
	if first.Code != http.StatusOK {
		t.Fatalf("first login: %d %s", first.Code, first.Body)
	}
	second := call(false)
	var a, b struct {
		Username string `json:"username"`
	}
	_ = json.Unmarshal(first.Body.Bytes(), &a)
	_ = json.Unmarshal(second.Body.Bytes(), &b)
	if second.Code != http.StatusOK || a.Username == "" || a.Username != b.Username {
		t.Fatalf("second login should reuse the account: %q vs %q (%d)", a.Username, b.Username, second.Code)
	}
}
