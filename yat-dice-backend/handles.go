package main

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

// Kullanıcı adları "Ad#1234" biçiminde saklanır. Aynı ad farklı etiketle birden fazla kişide olabilir;
// arkadaş eklerken ve şikâyet ederken etiketli tam ad kullanılır.

var errAmbiguousName = errors.New("bu ada sahip birden fazla oyuncu var")

// splitHandle "Ad#1234" değerini ad ve etikete ayırır; etiket yoksa boş döner.
func splitHandle(handle string) (name, tag string) {
	if i := strings.LastIndex(handle, "#"); i >= 0 {
		return handle[:i], handle[i+1:]
	}
	return handle, ""
}

func makeHandle(name, tag string) string {
	return name + "#" + tag
}

// handleTagCandidate ilk denemelerde 4, sonrasında 6 haneli etiket üretir (çok kullanılan adlar için).
func handleTagCandidate(attempt int) string {
	if attempt < 8 {
		return fmt.Sprintf("%04d", secureRandInt(9000)+1000)
	}
	return fmt.Sprintf("%06d", secureRandInt(900000)+100000)
}

// validDisplayName etiketten önceki adı kontrol eder.
func validDisplayName(name string) bool {
	n := len([]rune(name))
	return n >= 2 && n <= 30 && !strings.ContainsAny(name, "#\n\r\t")
}

type rowQuerier interface {
	QueryRow(query string, args ...interface{}) *sql.Row
}

// insertWithHandle verilen adla, boş bir etiket bulana kadar kaydı dener. insert, aday kullanıcı adıyla
// satırı ekleyip id döndürmelidir; benzersizlik ihlalinde hata dönerse yeni etiketle tekrar denenir.
func insertWithHandle(name string, insert func(handle string) (int, error)) (int, string, error) {
	var lastErr error
	for attempt := 0; attempt < 14; attempt++ {
		handle := makeHandle(name, handleTagCandidate(attempt))
		id, err := insert(handle)
		if err == nil {
			return id, handle, nil
		}
		lastErr = err
		if !isUniqueViolation(err) {
			return 0, "", err
		}
	}
	return 0, "", lastErr
}

func isUniqueViolation(err error) bool {
	return err != nil && (strings.Contains(err.Error(), "23505") || strings.Contains(strings.ToLower(err.Error()), "unique") || strings.Contains(strings.ToLower(err.Error()), "duplicate"))
}

// resolveHandle girilen adı kullanıcıya çevirir. "Ad#1234" tam eşleşir; etiketsiz ad yalnızca tek bir
// oyuncuya aitse kabul edilir, birden fazla oyuncuda varsa errAmbiguousName döner.
func resolveHandle(q rowQuerier, input string) (int, string, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return 0, "", sql.ErrNoRows
	}
	var id int
	var handle string
	if strings.Contains(input, "#") {
		err := q.QueryRow(`SELECT id, username FROM users WHERE LOWER(username) = LOWER($1)`, input).Scan(&id, &handle)
		return id, handle, err
	}
	var count int
	err := q.QueryRow(`
		SELECT COUNT(*), COALESCE(MIN(id), 0), COALESCE(MIN(username), '')
		FROM users WHERE LOWER(split_part(username, '#', 1)) = LOWER($1)
	`, input).Scan(&count, &id, &handle)
	if err != nil {
		return 0, "", err
	}
	switch {
	case count == 0:
		return 0, "", sql.ErrNoRows
	case count > 1:
		return 0, "", errAmbiguousName
	}
	return id, handle, nil
}
