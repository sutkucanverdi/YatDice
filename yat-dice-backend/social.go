package main

import (
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ─── OYUNCU PROFİLİ VE ARKADAŞ MESAJLARI ──────────────────────────────────────

const (
	maxDirectMessageRunes  = 300
	directMessageRetention = 30 * 24 * time.Hour
	directMessageCooldown  = 700 * time.Millisecond
)

var socialMigrations = []string{
	`CREATE TABLE IF NOT EXISTS direct_messages (
		id SERIAL PRIMARY KEY,
		sender_id INT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
		receiver_id INT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
		text VARCHAR(600) NOT NULL,
		created_at TIMESTAMPTZ DEFAULT NOW(),
		read_at TIMESTAMPTZ
	);`,
	`CREATE INDEX IF NOT EXISTS direct_messages_pair ON direct_messages (sender_id, receiver_id, id);`,
	`CREATE INDEX IF NOT EXISTS direct_messages_unread ON direct_messages (receiver_id) WHERE read_at IS NULL;`,
}

// Arkadaş mesajları 30 gün saklanır; hesap silinince veritabanı tarafından silinir.
func pruneDirectMessages() {
	if _, err := db.Exec(`DELETE FROM direct_messages WHERE created_at < $1`, time.Now().Add(-directMessageRetention)); err != nil {
		log.Println("Eski arkadaş mesajları temizlenemedi:", err)
	}
}

func startSocialMaintenance() {
	pruneDirectMessages()
	go func() {
		for range time.NewTicker(time.Hour).C {
			pruneDirectMessages()
		}
	}()
}

// GET /players/profile?name=Ad#1234 : Başka bir oyuncunun herkese açık profili
func handlePlayerProfile(w http.ResponseWriter, r *http.Request) {
	if setupCORS(&w, r) {
		return
	}
	if !applySecurityAndRateLimit(w, r, false) {
		return
	}
	myID, ok := requireUser(w, r)
	if !ok {
		return
	}
	targetID, handle, err := resolveHandle(db, r.URL.Query().Get("name"))
	if errors.Is(err, errAmbiguousName) {
		httpError(w, r, "Bu adı kullanan birden fazla oyuncu var. Etiketiyle birlikte yaz (ör. Ad#1234).", http.StatusConflict)
		return
	}
	// Engellenen veya kapatılmış oyuncu yokmuş gibi gösterilir
	if err != nil || (targetID != myID && isBlockedEither(myID, targetID)) || isUserBanned(targetID) {
		httpError(w, r, "Oyuncu bulunamadı", http.StatusNotFound)
		return
	}

	var p struct {
		Username     string `json:"username"`
		Avatar       string `json:"avatar"`
		Country      string `json:"country"`
		XP           int    `json:"xp"`
		Wins         int    `json:"wins"`
		GamesPlayed  int    `json:"gamesPlayed"`
		HighScore    int    `json:"highScore"`
		SelectedDice string `json:"selectedDice"`
		IsMe         bool   `json:"isMe"`
		Friendship   string `json:"friendship"` // none, friends, outgoing, incoming
		UserID       int    `json:"userId"`
		IsOnline     bool   `json:"isOnline"`
		InGame       bool   `json:"inGame"`
	}
	p.Username = handle
	p.UserID = targetID
	err = db.QueryRow(`
		SELECT COALESCE(avatar, ''), COALESCE(country, 'TR'), COALESCE(xp, 0), COALESCE(wins, 0),
		       COALESCE(games_played, 0), COALESCE(high_score, 0), COALESCE(selected_dice, 'ivory')
		FROM profiles WHERE user_id = $1`, targetID).
		Scan(&p.Avatar, &p.Country, &p.XP, &p.Wins, &p.GamesPlayed, &p.HighScore, &p.SelectedDice)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		httpError(w, r, "Profil okunamadı", http.StatusInternalServerError)
		return
	}
	p.IsMe = targetID == myID
	p.Friendship = friendshipState(myID, targetID)
	if p.Friendship == "friends" || p.IsMe {
		p.IsOnline, p.InGame = getUserOnlineState(targetID)
	}
	writeJSON(w, p)
}

func friendshipState(me, other int) string {
	if me == other {
		return "none"
	}
	var requester int
	var status string
	err := db.QueryRow(`
		SELECT requester_id, status FROM friendships
		WHERE (requester_id = $1 AND addressee_id = $2) OR (requester_id = $2 AND addressee_id = $1)
		LIMIT 1`, me, other).Scan(&requester, &status)
	switch {
	case err != nil:
		return "none"
	case status == "accepted":
		return "friends"
	case requester == me:
		return "outgoing"
	default:
		return "incoming"
	}
}

// unreadMessageCounts arkadaş başına okunmamış mesaj sayısını döner.
func unreadMessageCounts(userID int) map[int]int {
	counts := map[int]int{}
	rows, err := db.Query(`SELECT sender_id, COUNT(*) FROM direct_messages WHERE receiver_id = $1 AND read_at IS NULL GROUP BY sender_id`, userID)
	if err != nil {
		return counts
	}
	defer rows.Close()
	for rows.Next() {
		var sender, count int
		if rows.Scan(&sender, &count) == nil {
			counts[sender] = count
		}
	}
	return counts
}

type directMessage struct {
	ID   int       `json:"id"`
	Mine bool      `json:"mine"`
	Text string    `json:"text"`
	At   time.Time `json:"at"`
}

// Mesajlaşma yalnızca kabul edilmiş ve birbirini engellememiş arkadaşlar arasında açıktır.
func canMessage(me, other int) bool {
	return me != other && areFriends(me, other) && !isBlockedEither(me, other)
}

// GET /messages?with=<userId>&after=<id> : Arkadaşla mesajları getirir ve gelenleri okundu yapar
func handleGetMessages(w http.ResponseWriter, r *http.Request) {
	if setupCORS(&w, r) {
		return
	}
	if !applySecurityAndRateLimit(w, r, false) {
		return
	}
	myID, ok := requireUser(w, r)
	if !ok {
		return
	}
	other, _ := strconv.Atoi(r.URL.Query().Get("with"))
	after, _ := strconv.Atoi(r.URL.Query().Get("after"))
	if !canMessage(myID, other) {
		httpError(w, r, "Yalnızca arkadaşlarınla mesajlaşabilirsin", http.StatusForbidden)
		return
	}
	touchUserPresence(myID)

	rows, err := db.Query(`
		SELECT id, sender_id, text, created_at FROM (
			SELECT id, sender_id, text, created_at FROM direct_messages
			WHERE ((sender_id = $1 AND receiver_id = $2) OR (sender_id = $2 AND receiver_id = $1)) AND id > $3
			ORDER BY id DESC LIMIT 60
		) recent ORDER BY id`, myID, other, after)
	if err != nil {
		httpError(w, r, "Mesajlar alınamadı", http.StatusInternalServerError)
		return
	}
	defer rows.Close()
	messages := []directMessage{}
	for rows.Next() {
		var m directMessage
		var sender int
		if rows.Scan(&m.ID, &sender, &m.Text, &m.At) == nil {
			m.Mine = sender == myID
			messages = append(messages, m)
		}
	}
	_, _ = db.Exec(`UPDATE direct_messages SET read_at = NOW() WHERE receiver_id = $1 AND sender_id = $2 AND read_at IS NULL`, myID, other)
	writeJSON(w, map[string]interface{}{"messages": messages})
}

var (
	lastDirectMessageMu sync.Mutex
	lastDirectMessageAt = map[int]time.Time{}
)

// POST /messages {to, text} : Arkadaşa mesaj gönderir
func handleSendMessage(w http.ResponseWriter, r *http.Request) {
	if setupCORS(&w, r) {
		return
	}
	if !applySecurityAndRateLimit(w, r, false) {
		return
	}
	myID, ok := requireUser(w, r)
	if !ok {
		return
	}
	if r.Method != http.MethodPost {
		httpError(w, r, "Geçersiz istek", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		To   int    `json:"to"`
		Text string `json:"text"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&req); err != nil {
		httpError(w, r, "Geçersiz istek", http.StatusBadRequest)
		return
	}
	text := strings.TrimSpace(strings.ReplaceAll(req.Text, "\r", ""))
	if text == "" {
		httpError(w, r, "Mesaj boş olamaz", http.StatusBadRequest)
		return
	}
	if runes := []rune(text); len(runes) > maxDirectMessageRunes {
		text = string(runes[:maxDirectMessageRunes])
	}
	if containsProfanity(text) {
		httpError(w, r, "Mesajın topluluk kurallarına aykırı ifadeler içeriyor", http.StatusBadRequest)
		return
	}
	if !canMessage(myID, req.To) {
		httpError(w, r, "Yalnızca arkadaşlarınla mesajlaşabilirsin", http.StatusForbidden)
		return
	}

	lastDirectMessageMu.Lock()
	if time.Since(lastDirectMessageAt[myID]) < directMessageCooldown {
		lastDirectMessageMu.Unlock()
		httpError(w, r, "Çok hızlı yazıyorsun, biraz bekle", http.StatusTooManyRequests)
		return
	}
	lastDirectMessageAt[myID] = time.Now()
	lastDirectMessageMu.Unlock()

	var m directMessage
	err := db.QueryRow(`INSERT INTO direct_messages (sender_id, receiver_id, text) VALUES ($1, $2, $3) RETURNING id, created_at`,
		myID, req.To, text).Scan(&m.ID, &m.At)
	if err != nil {
		httpError(w, r, "Mesaj gönderilemedi", http.StatusInternalServerError)
		return
	}
	touchUserPresence(myID)
	m.Mine = true
	m.Text = text
	writeJSON(w, m)
}

// directMessageEvidence şikâyet edilen oyuncunun şikâyet edene son 7 günde yazdığı mesajları döner.
func directMessageEvidence(targetID, reporterID int) []chatRecord {
	records := []chatRecord{}
	rows, err := db.Query(`
		SELECT text, created_at FROM direct_messages
		WHERE sender_id = $1 AND receiver_id = $2 AND created_at > NOW() - INTERVAL '7 days'
		ORDER BY id DESC LIMIT 20`, targetID, reporterID)
	if err != nil {
		return records
	}
	defer rows.Close()
	for rows.Next() {
		var rec chatRecord
		if rows.Scan(&rec.Text, &rec.At) == nil {
			records = append([]chatRecord{rec}, records...)
		}
	}
	return records
}

// reportEvidence maç sohbetini ve arkadaş mesajlarını tek kanıt listesinde birleştirir.
func reportEvidence(targetID, reporterID int) string {
	var records []chatRecord
	_ = json.Unmarshal([]byte(recentChatEvidence(targetID)), &records)
	records = append(records, directMessageEvidence(targetID, reporterID)...)
	encoded, err := json.Marshal(records)
	if err != nil {
		return "[]"
	}
	return string(encoded)
}
