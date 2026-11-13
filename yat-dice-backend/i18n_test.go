package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

func TestParseLang(t *testing.T) {
	cases := []struct{ query, header, want string }{
		{"", "", langTR},
		{"", "tr-TR,tr;q=0.9", langTR},
		{"", "en-US,en;q=0.9", langEN},
		{"", "de-DE", langEN},
		{"tr", "en-US", langTR},
		{"en", "tr-TR", langEN},
		{"fr", "tr", langTR},
	}
	for _, c := range cases {
		if got := parseLang(c.query, c.header); got != c.want {
			t.Errorf("parseLang(%q, %q) = %q, want %q", c.query, c.header, got, c.want)
		}
	}
}

func TestHTTPErrorIsLocalized(t *testing.T) {
	request := httptest.NewRequest(http.MethodPost, "/login", nil)
	request.Header.Set("Accept-Language", "en")
	recorder := httptest.NewRecorder()
	httpError(recorder, request, "Kullanıcı bulunamadı veya şifre yanlış", http.StatusUnauthorized)
	if body := strings.TrimSpace(recorder.Body.String()); body != "User not found or wrong password" {
		t.Fatalf("got %q", body)
	}
	recorder = httptest.NewRecorder()
	httpError(recorder, httptest.NewRequest(http.MethodPost, "/login", nil), "Kullanıcı bulunamadı veya şifre yanlış", http.StatusUnauthorized)
	if body := strings.TrimSpace(recorder.Body.String()); body != "Kullanıcı bulunamadı veya şifre yanlış" {
		t.Fatalf("Turkish default lost: %q", body)
	}
	if got := Lf(request, "Çok fazla hatalı giriş denemesi. Lütfen %d saniye sonra tekrar deneyin.", 30); got != "Too many failed sign-in attempts. Please try again in 30 seconds." {
		t.Fatalf("formatted message: %q", got)
	}
}

func TestLegalPagesInEnglish(t *testing.T) {
	t.Setenv("LEGAL_CONTACT_EMAIL", "support@example.com")
	for path, handler := range map[string]http.HandlerFunc{
		"/privacy?lang=en": handlePrivacyHTML,
		"/terms?lang=en":   handleTermsHTML,
		"/support?lang=en": handleSupportHTML,
	} {
		recorder := httptest.NewRecorder()
		handler(recorder, httptest.NewRequest(http.MethodGet, path, nil))
		body := recorder.Body.String()
		if !strings.Contains(body, `<html lang="en">`) || !strings.Contains(body, "mailto:support@example.com") || strings.Contains(body, "Gizlilik") {
			t.Errorf("%s is not a full English page", path)
		}
	}
	recorder := httptest.NewRecorder()
	handlePrivacyHTML(recorder, httptest.NewRequest(http.MethodGet, "/privacy", nil))
	if !strings.Contains(recorder.Body.String(), "Gizlilik Politikası") {
		t.Error("Turkish privacy page missing without a language")
	}
}

func TestEnglishTablesCoverTurkishKeys(t *testing.T) {
	for tr, en := range enMessages {
		if strings.TrimSpace(en) == "" {
			t.Errorf("empty English text for %q", tr)
		}
	}
}

// Her httpError / L / localize çağrısındaki Türkçe metnin İngilizce karşılığı olmalı.
func TestEveryLocalizedMessageHasEnglish(t *testing.T) {
	fset := token.NewFileSet()
	for _, file := range []string{"main.go", "compliance.go", "apple.go"} {
		parsed, err := parser.ParseFile(fset, file, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		ast.Inspect(parsed, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			name := ""
			switch fn := call.Fun.(type) {
			case *ast.Ident:
				name = fn.Name
			}
			index := map[string]int{"httpError": 2, "L": 1, "Lf": 1, "localize": 1}[name]
			if index == 0 || len(call.Args) <= index {
				return true
			}
			lit, ok := call.Args[index].(*ast.BasicLit)
			if !ok {
				return true
			}
			text, err := strconv.Unquote(lit.Value)
			if err != nil {
				return true
			}
			if _, ok := enMessages[text]; !ok {
				t.Errorf("%s: no English text for %q", fset.Position(lit.Pos()), text)
			}
			return true
		})
	}
}
