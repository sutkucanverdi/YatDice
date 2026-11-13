package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

func TestResolveJWTSecret(t *testing.T) {
	strong := strings.Repeat("k", minJWTSecretLength)
	cases := []struct {
		name    string
		secret  string
		dev     bool
		wantErr bool
	}{
		{"production rejects empty", "", false, true},
		{"production rejects default", defaultDevJWTSecret, false, true},
		{"production rejects short", "too-short", false, true},
		{"production accepts strong", strong, false, false},
		{"development accepts default", defaultDevJWTSecret, true, false},
		{"development generates when empty", "", true, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			key, err := resolveJWTSecret(tc.secret, tc.dev)
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tc.wantErr)
			}
			if err == nil && len(key) == 0 {
				t.Fatal("empty key")
			}
		})
	}
}

func TestIsDevelopmentRequiresExplicitValue(t *testing.T) {
	for value, want := range map[string]bool{"": false, "production": false, "staging": false, "development": true, "Development": true} {
		t.Setenv("APP_ENV", value)
		if got := isDevelopment(); got != want {
			t.Errorf("APP_ENV=%q: got %v, want %v", value, got, want)
		}
	}
}

func TestClientIPFrom(t *testing.T) {
	cases := []struct {
		name    string
		xff     string
		proxies int
		want    string
	}{
		{"no proxy ignores spoofed header", "1.2.3.4", 0, "9.9.9.9"},
		{"one proxy uses the address it appended", "1.2.3.4, 5.6.7.8", 1, "5.6.7.8"},
		{"two proxies count from the right", "1.2.3.4, 5.6.7.8, 10.0.0.1", 2, "5.6.7.8"},
		{"short chain falls back to peer", "5.6.7.8", 2, "9.9.9.9"},
		{"missing header falls back to peer", "", 1, "9.9.9.9"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := clientIPFrom("9.9.9.9:4000", tc.xff, tc.proxies); got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestVerifyGoogleIDToken(t *testing.T) {
	valid := `{"iss":"https://accounts.google.com","aud":"web-id","sub":"123","email":"A@Example.com","email_verified":"true","name":"Ada"}`
	cases := []struct {
		name    string
		status  int
		body    string
		wantErr bool
	}{
		{"valid token", http.StatusOK, valid, false},
		{"rejected by google", http.StatusBadRequest, `{"error":"invalid_token"}`, true},
		{"token for another app", http.StatusOK, strings.Replace(valid, "web-id", "other-app", 1), true},
		{"wrong issuer", http.StatusOK, strings.Replace(valid, "https://accounts.google.com", "https://evil.example", 1), true},
		{"unverified email", http.StatusOK, strings.Replace(valid, `"email_verified":"true"`, `"email_verified":"false"`, 1), true},
		{"missing subject", http.StatusOK, strings.Replace(valid, `"sub":"123"`, `"sub":""`, 1), true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var gotToken string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotToken = r.URL.Query().Get("id_token")
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer srv.Close()
			original := googleTokenInfoURL
			googleTokenInfoURL = srv.URL
			defer func() { googleTokenInfoURL = original }()

			identity, err := verifyGoogleIDToken("tok&en=1", []string{"android-id", "web-id"})
			if gotToken != "tok&en=1" {
				t.Fatalf("credential was not URL-escaped: %q", gotToken)
			}
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tc.wantErr)
			}
			if err == nil && (identity.Sub != "123" || identity.Email != "a@example.com") {
				t.Fatalf("unexpected identity: %+v", identity)
			}
		})
	}
}

// TEST_DATABASE_URL tanımlıysa boş bir test veritabanında çalışır.
func openTestDB(t *testing.T) {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL tanımlı değil")
	}
	conn, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	db = conn
	t.Cleanup(func() { _ = conn.Close() })
	createTables()
}

func TestGoogleAuthAccountRules(t *testing.T) {
	openTestDB(t)
	jwtKey = []byte(strings.Repeat("k", minJWTSecretLength))
	t.Setenv("GOOGLE_CLIENT_ID", "web-id")

	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	sub := "sub-" + suffix
	email := "user" + suffix + "@example.com"
	tokenInfo := fmt.Sprintf(`{"iss":"accounts.google.com","aud":"web-id","sub":%q,"email":%q,"email_verified":"true","name":"Ada"}`, sub, email)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(tokenInfo))
	}))
	defer srv.Close()
	original := googleTokenInfoURL
	googleTokenInfoURL = srv.URL
	defer func() { googleTokenInfoURL = original }()

	call := func(body string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		handleGoogleAuth(rec, httptest.NewRequest(http.MethodPost, "/auth/google", strings.NewReader(body)))
		return rec
	}

	if rec := call(`{"email":"` + email + `","googleId":"x"}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("request without credential: got %d", rec.Code)
	}

	first := call(`{"credential":"t","acceptedTerms":true}`)
	if first.Code != http.StatusOK {
		t.Fatalf("first google login: got %d: %s", first.Code, first.Body)
	}
	second := call(`{"credential":"t","acceptedTerms":true}`)
	var a, b struct {
		Username string `json:"username"`
	}
	_ = json.Unmarshal(first.Body.Bytes(), &a)
	_ = json.Unmarshal(second.Body.Bytes(), &b)
	if second.Code != http.StatusOK || a.Username == "" || a.Username != b.Username {
		t.Fatalf("second login should reuse the account: %q vs %q (%d)", a.Username, b.Username, second.Code)
	}

	// Doğrulanmamış /register e-postası başka bir Google hesabına bağlanmamalı
	claimed := "claimed" + suffix + "@example.com"
	if _, err := db.Exec(`INSERT INTO users (username, password_hash, email) VALUES ($1, 'x', $2)`, "pw"+suffix, claimed); err != nil {
		t.Fatal(err)
	}
	tokenInfo = fmt.Sprintf(`{"iss":"accounts.google.com","aud":"web-id","sub":"other-%s","email":%q,"email_verified":"true"}`, suffix, claimed)
	if rec := call(`{"credential":"t","acceptedTerms":true}`); rec.Code != http.StatusConflict {
		t.Fatalf("existing email should not auto-link: got %d", rec.Code)
	}
}

func TestUsernamesAreUniqueIgnoringCase(t *testing.T) {
	openTestDB(t)
	name := fmt.Sprintf("Case%d", time.Now().UnixNano())
	if _, err := db.Exec(`INSERT INTO users (username, password_hash) VALUES ($1, 'x')`, name); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO users (username, password_hash) VALUES ($1, 'x')`, strings.ToLower(name)); err == nil {
		t.Fatal("case-variant username was accepted")
	}
}

func TestWSMessageBudget(t *testing.T) {
	var budget wsMessageBudget
	now := time.Now()
	for i := 0; i < maxWSMessagesPerSecond; i++ {
		if !budget.allow(now) {
			t.Fatalf("message %d rejected inside the budget", i)
		}
	}
	if budget.allow(now) {
		t.Fatal("message over the budget accepted")
	}
	if !budget.allow(now.Add(time.Second)) {
		t.Fatal("budget did not reset after a second")
	}
}

func TestIsForbiddenUsername(t *testing.T) {
	for name, want := range map[string]bool{
		"🤖 YatBot": true, "yatbot_tr": true, "fuck": true, "SalakAdam": true, "Oyuncu_Orospu": true,
		"Ada": false, "KlasikOyuncu": false, "Götürücü": false,
	} {
		if got := isForbiddenUsername(name); got != want {
			t.Errorf("%q: got %v, want %v", name, got, want)
		}
	}
}

func TestContainsProfanity(t *testing.T) {
	for text, want := range map[string]bool{
		"salaksın sen": true, "siktir git": true, "AMK": true, "göt!": true, "aq.": true,
		"maçı götürdüm": false, "klasik açılış": false, "aqua renk": false, "iyi oyundu": false, "Dickens okudum": false,
	} {
		if got := containsProfanity(text); got != want {
			t.Errorf("%q: got %v, want %v", text, got, want)
		}
	}
}
