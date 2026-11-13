package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"
)

func TestSplitHandle(t *testing.T) {
	for in, want := range map[string][2]string{
		"Utku#4821":   {"Utku", "4821"},
		"Ali#Veli#12": {"Ali#Veli", "12"},
		"🤖 YatBot":    {"🤖 YatBot", ""},
	} {
		name, tag := splitHandle(in)
		if name != want[0] || tag != want[1] {
			t.Errorf("splitHandle(%q) = %q, %q", in, name, tag)
		}
	}
	if validDisplayName("a#b") || validDisplayName("x") || !validDisplayName("Ayşe") {
		t.Error("display name validation is wrong")
	}
}

func postJSON(t *testing.T, handler http.HandlerFunc, path string, body interface{}, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	raw, _ := json.Marshal(body)
	request := httptest.NewRequest(http.MethodPost, path, strings.NewReader(string(raw)))
	request.Header.Set("Content-Type", "application/json")
	// Her istek ayrı bir adresten gelsin; hız sınırı diğer testleri etkilemesin
	request.RemoteAddr = fmt.Sprintf("10.%d.%d.%d:4000", secureRandInt(250), secureRandInt(250), secureRandInt(250))
	for k, v := range headers {
		request.Header.Set(k, v)
	}
	recorder := httptest.NewRecorder()
	handler(recorder, request)
	return recorder
}

func TestSameNameGetsDifferentTags(t *testing.T) {
	openTestDB(t)
	name := fmt.Sprintf("Ayni%d", time.Now().UnixNano()%1_000_000)
	handles := []string{}
	for i := 0; i < 2; i++ {
		rec := postJSON(t, handleRegister, "/register", map[string]interface{}{"username": name, "password": "secret123", "acceptedTerms": true}, nil)
		if rec.Code != http.StatusCreated {
			t.Fatalf("register %d: %d %s", i, rec.Code, rec.Body)
		}
		var resp map[string]string
		_ = json.Unmarshal(rec.Body.Bytes(), &resp)
		if !regexp.MustCompile(`^` + name + `#\d{4,6}$`).MatchString(resp["username"]) {
			t.Fatalf("unexpected handle %q", resp["username"])
		}
		handles = append(handles, resp["username"])
	}
	if strings.EqualFold(handles[0], handles[1]) {
		t.Fatalf("both accounts got %q", handles[0])
	}

	// Etiketsiz ad iki hesaba ait olduğu için giriş belirsizdir
	rec := postJSON(t, handleLogin, "/login", map[string]string{"username": name, "password": "secret123"}, map[string]string{"Accept-Language": "en"})
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "Name#1234") {
		t.Fatalf("ambiguous login: %d %s", rec.Code, rec.Body)
	}
	// Etiketle giriş çalışır ve doğru hesabı döndürür
	rec = postJSON(t, handleLogin, "/login", map[string]string{"username": strings.ToLower(handles[1]), "password": "secret123"}, nil)
	var login map[string]interface{}
	_ = json.Unmarshal(rec.Body.Bytes(), &login)
	if rec.Code != http.StatusOK || login["username"] != handles[1] {
		t.Fatalf("tag login: %d %s", rec.Code, rec.Body)
	}
	if _, _, err := resolveHandle(db, name); !errors.Is(err, errAmbiguousName) {
		t.Fatalf("resolveHandle should be ambiguous, got %v", err)
	}
}

func TestUniqueNameLogsInWithoutTag(t *testing.T) {
	openTestDB(t)
	name := fmt.Sprintf("Tek%d", time.Now().UnixNano()%1_000_000)
	rec := postJSON(t, handleRegister, "/register", map[string]interface{}{"username": name, "password": "secret123", "acceptedTerms": true}, nil)
	if rec.Code != http.StatusCreated {
		t.Fatalf("register: %d %s", rec.Code, rec.Body)
	}
	rec = postJSON(t, handleLogin, "/login", map[string]string{"username": name, "password": "secret123"}, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("login without tag: %d %s", rec.Code, rec.Body)
	}
}

func TestLegacyUsernamesGetTags(t *testing.T) {
	openTestDB(t)
	name := fmt.Sprintf("Eski%d", time.Now().UnixNano()%1_000_000)
	id := insertTestUser(t, name)
	if _, err := db.Exec(`UPDATE users SET username = username || '#' || (floor(random() * 9000) + 1000)::int WHERE username NOT LIKE '%#%'`); err != nil {
		t.Fatal(err)
	}
	var handle string
	_ = db.QueryRow(`SELECT username FROM users WHERE id = $1`, id).Scan(&handle)
	if !regexp.MustCompile(`^` + name + `#\d{4}$`).MatchString(handle) {
		t.Fatalf("legacy user became %q", handle)
	}
	if gotID, _, err := resolveHandle(db, name); err != nil || gotID != id {
		t.Fatalf("legacy name lookup: %d %v", gotID, err)
	}
}
