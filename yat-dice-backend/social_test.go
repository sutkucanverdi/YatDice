package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func authedRequest(t *testing.T, method, target, body string, userID int) *http.Request {
	t.Helper()
	token, err := issueJWTForUser(userID, 1)
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(method, target, strings.NewReader(body))
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Content-Type", "application/json")
	request.RemoteAddr = fmt.Sprintf("10.%d.%d.%d:4000", secureRandInt(250), secureRandInt(250), secureRandInt(250))
	return request
}

func makeFriends(t *testing.T, a, b int) {
	t.Helper()
	if _, err := db.Exec(`INSERT INTO friendships (requester_id, addressee_id, status) VALUES ($1, $2, 'accepted')`, a, b); err != nil {
		t.Fatal(err)
	}
}

func TestMessagesOnlyBetweenFriends(t *testing.T) {
	openTestDB(t)
	suffix := time.Now().UnixNano() % 1_000_000
	a := insertTestUser(t, fmt.Sprintf("dmA%d#1111", suffix))
	b := insertTestUser(t, fmt.Sprintf("dmB%d#2222", suffix))

	send := func(from, to int, text string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		handleSendMessage(rec, authedRequest(t, http.MethodPost, "/messages", fmt.Sprintf(`{"to":%d,"text":%q}`, to, text), from))
		return rec
	}
	if rec := send(a, b, "merhaba"); rec.Code != http.StatusForbidden {
		t.Fatalf("non-friends could message: %d", rec.Code)
	}

	makeFriends(t, a, b)
	if rec := send(a, b, "merhaba"); rec.Code != http.StatusOK {
		t.Fatalf("friend message failed: %d %s", rec.Code, rec.Body)
	}
	if rec := send(a, b, "ikinci"); rec.Code != http.StatusTooManyRequests {
		t.Fatalf("cooldown not applied: %d", rec.Code)
	}
	time.Sleep(directMessageCooldown)
	if rec := send(a, b, "salaksın"); rec.Code != http.StatusBadRequest {
		t.Fatalf("profanity accepted: %d", rec.Code)
	}
	if got := unreadMessageCounts(b)[a]; got != 1 {
		t.Fatalf("unread for b = %d, want 1", got)
	}

	rec := httptest.NewRecorder()
	handleGetMessages(rec, authedRequest(t, http.MethodGet, fmt.Sprintf("/messages?with=%d", a), "", b))
	var resp struct{ Messages []directMessage }
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if rec.Code != http.StatusOK || len(resp.Messages) != 1 || resp.Messages[0].Text != "merhaba" || resp.Messages[0].Mine {
		t.Fatalf("read messages: %d %s", rec.Code, rec.Body)
	}
	if got := unreadMessageCounts(b)[a]; got != 0 {
		t.Fatalf("messages not marked read: %d", got)
	}

	// Şikâyet kanıtı arkadaş mesajını içerir
	if evidence := reportEvidence(a, b); !strings.Contains(evidence, "merhaba") {
		t.Fatalf("evidence lacks the message: %s", evidence)
	}

	// Engellendikten sonra mesajlaşma kapanır
	if _, err := db.Exec(`INSERT INTO user_blocks (blocker_id, blocked_id, blocked_username) VALUES ($1, $2, 'x')`, b, a); err != nil {
		t.Fatal(err)
	}
	time.Sleep(directMessageCooldown)
	if rec := send(a, b, "hala orada mısın"); rec.Code != http.StatusForbidden {
		t.Fatalf("blocked user could message: %d", rec.Code)
	}

	// Hesap silinince mesajlar da silinir
	if err := deleteUserData(a); err != nil {
		t.Fatal(err)
	}
	var left int
	_ = db.QueryRow(`SELECT COUNT(*) FROM direct_messages WHERE sender_id = $1 OR receiver_id = $1`, a).Scan(&left)
	if left != 0 {
		t.Fatalf("%d messages left after deletion", left)
	}
}

func TestPlayerProfile(t *testing.T) {
	openTestDB(t)
	suffix := time.Now().UnixNano() % 1_000_000
	me := insertTestUser(t, fmt.Sprintf("pfMe%d#1111", suffix))
	otherName := fmt.Sprintf("pfOther%d#2222", suffix)
	other := insertTestUser(t, otherName)
	if _, err := db.Exec(`INSERT INTO profiles (user_id, wins, games_played, high_score, country) VALUES ($1, 7, 12, 240, 'DE')`, other); err != nil {
		t.Fatal(err)
	}

	get := func() (*httptest.ResponseRecorder, map[string]interface{}) {
		rec := httptest.NewRecorder()
		handlePlayerProfile(rec, authedRequest(t, http.MethodGet, "/players/profile?name="+url.QueryEscape(otherName), "", me))
		var body map[string]interface{}
		_ = json.Unmarshal(rec.Body.Bytes(), &body)
		return rec, body
	}
	rec, body := get()
	if rec.Code != http.StatusOK || body["wins"] != float64(7) || body["gamesPlayed"] != float64(12) || body["country"] != "DE" || body["friendship"] != "none" {
		t.Fatalf("profile: %d %s", rec.Code, rec.Body)
	}
	if _, ok := body["email"]; ok {
		t.Fatal("profile leaks email")
	}
	if _, err := db.Exec(`INSERT INTO friendships (requester_id, addressee_id, status) VALUES ($1, $2, 'pending')`, me, other); err != nil {
		t.Fatal(err)
	}
	if _, body = get(); body["friendship"] != "outgoing" {
		t.Fatalf("friendship = %v, want outgoing", body["friendship"])
	}
	if _, err := db.Exec(`INSERT INTO user_blocks (blocker_id, blocked_id, blocked_username) VALUES ($1, $2, 'x')`, other, me); err != nil {
		t.Fatal(err)
	}
	if rec, _ = get(); rec.Code != http.StatusNotFound {
		t.Fatalf("blocked profile visible: %d", rec.Code)
	}
}
