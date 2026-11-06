package main

import (
	"crypto/rand"
	"crypto/subtle"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"math/big"
	"net"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/gorilla/websocket"
	_ "github.com/lib/pq"
	"golang.org/x/crypto/bcrypt"
)

const (
	TurnDurationSeconds = 30
	RoomExpiryMinutes   = 10
	PongWait            = 60 * time.Second
	PingPeriod          = (PongWait * 9) / 10
	ReconnectGrace      = 60 * time.Second // Kopan oyuncunun maça dönmek için süresi
	BcryptCost          = 10               // Cost 14 DoS riskine yol açtığı için 10'a çekildi

	defaultDevJWTSecret    = "dev-secret-change-in-production"
	minJWTSecretLength     = 32
	maxWSMessagesPerSecond = 20 // Tek bağlantıdan saniyede kabul edilen en fazla mesaj
)

var allowedCategories = map[string]bool{
	"ones": true, "twos": true, "threes": true,
	"fours": true, "fives": true, "sixes": true,
	"three": true, "four": true, "full": true,
	"small": true, "large": true, "yacht": true, "chance": true,
}

var upperCategories = []string{"ones", "twos", "threes", "fours", "fives", "sixes"}

var dicePrices = map[string]int{
	"ivory":    0,
	"wood":     250,
	"marble":   600,
	"obsidian": 1500,
	"ruby":     2500,
	"gold":     4000,
}

var allowedBets = map[int]bool{
	0: true, 50: true, 100: true, 250: true, 500: true,
}

func normalizeBet(raw interface{}) int {
	if f, ok := raw.(float64); ok {
		b := int(f)
		if allowedBets[b] {
			return b
		}
	}
	return 0
}

func isOriginAllowed(origin string) bool {
	if origin == "" {
		return true
	}
	allowedOrigin := strings.TrimSpace(os.Getenv("ALLOWED_ORIGIN"))
	if allowedOrigin == "*" {
		return true
	}
	if allowedOrigin != "" {
		for _, item := range strings.Split(allowedOrigin, ",") {
			if strings.EqualFold(strings.TrimSpace(item), origin) {
				return true
			}
		}
		return false
	}
	return strings.HasPrefix(origin, "http://localhost") ||
		strings.HasPrefix(origin, "https://localhost") ||
		strings.HasPrefix(origin, "http://127.0.0.1") ||
		strings.HasPrefix(origin, "capacitor://localhost") ||
		strings.HasPrefix(origin, "ionic://localhost")
}

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return isOriginAllowed(r.Header.Get("Origin"))
	},
}

type Client struct {
	ID          string
	UserID      int // Veritabanındaki kullanıcı ID'si (Misafir ise 0)
	Conn        *websocket.Conn
	WriteMu     sync.Mutex // Concurrent websocket write panic önleyici
	Room        *Room
	Username    string
	BetAmount   int
	LastEmojiAt time.Time
	LastChatAt  time.Time

	Disconnected bool // Room.Mu ile korunur: bağlantı koptu, geri dönüş bekleniyor
}

var profanityList = []string{
	"aptal", "salak", "gerizekalı", "küfür", "amk", "aq", "sik", "orospu", "piç", "yavşak", "göt", "ibne", "fuck", "shit", "bitch", "asshole", "dick", "cunt",
}

func containsProfanity(s string) bool {
	lower := strings.ToLower(s)
	for _, bw := range profanityList {
		if strings.Contains(lower, bw) {
			return true
		}
	}
	return false
}

// Kullanıcı adı küfür içeremez ve sistem botunu taklit edemez
func isForbiddenUsername(s string) bool {
	return containsProfanity(s) || strings.Contains(strings.ToLower(s), "yatbot")
}

func sanitizeChatMessage(raw string) string {
	s := strings.TrimSpace(raw)
	s = strings.ReplaceAll(s, "<", "")
	s = strings.ReplaceAll(s, ">", "")
	runes := []rune(s)
	if len(runes) > 120 {
		s = string(runes[:120])
	}
	if containsProfanity(s) {
		return "*** (Topluluk Kuralları gereği gizlendi)"
	}
	return s
}

// WriteJSON güvenli (thread-safe) WebSocket mesaj gönderimi sağlar
func (c *Client) WriteJSON(v interface{}) error {
	if c == nil || c.Conn == nil {
		return fmt.Errorf("bağlantı yok")
	}
	c.WriteMu.Lock()
	defer c.WriteMu.Unlock()
	_ = c.Conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
	return c.Conn.WriteJSON(v)
}

type PrivateRoomEntry struct {
	Host      *Client
	BetAmount int
	CreatedAt time.Time
}

type Room struct {
	ID            string
	Player1       *Client
	Player2       *Client
	IsBotMatch    bool
	BetAmount     int        // Bahis miktarı (her oyuncudan kesilen giriş ücreti; kazanan 2x alır)
	Mu            sync.Mutex // Oda içi oyun durumu kilidi (Global mutex darboğazını önler)
	Turn          string     // "p1" veya "p2"
	Round         int        // 1..13
	RollsLeft     int        // 3..0
	Dice          [5]int
	Locked        [5]bool
	P1Scores      map[string]int
	P2Scores      map[string]int
	P1YachtCount  int
	P2YachtCount  int
	GameOver      bool
	TurnDeadline  time.Time
	TurnTimerStop chan struct{}
}

type GameInvite struct {
	ID         int       `json:"id"`
	RoomCode   string    `json:"roomCode"`
	FromUserID int       `json:"fromUserId"`
	FromName   string    `json:"fromName"`
	CreatedAt  time.Time `json:"createdAt"`
}

var (
	waitingClients []*Client
	privateRooms   = make(map[string]PrivateRoomEntry)
	pendingInvites = make(map[int][]GameInvite) // targetUserID -> gelen oyun davetleri
	inviteSeq      int
	mu             sync.Mutex // Sadece eşleştirme (matchmaking) kuyruğunu ve davetleri korur
	roomCounter    int
	db             *sql.DB
	jwtKey         []byte
)

// Yalnızca APP_ENV açıkça "development" ise geliştirme modundayız.
// Değişken unutulursa sunucu güvenli tarafta (üretim kurallarıyla) çalışır.
func isDevelopment() bool {
	return strings.EqualFold(strings.TrimSpace(os.Getenv("APP_ENV")), "development")
}

// Üretimde güçlü bir JWT_SECRET zorunludur; geliştirmede boşsa geçici rastgele anahtar üretilir.
func resolveJWTSecret(secret string, dev bool) ([]byte, error) {
	if dev {
		if secret != "" {
			return []byte(secret), nil
		}
		key := make([]byte, 32)
		if _, err := rand.Read(key); err != nil {
			return nil, err
		}
		log.Println("⚠️ JWT_SECRET tanımlı değil: geçici anahtar üretildi, sunucu yeniden başlayınca oturumlar kapanır.")
		return key, nil
	}
	if secret == defaultDevJWTSecret || len(secret) < minJWTSecretLength {
		return nil, fmt.Errorf("JWT_SECRET en az %d karakter olmalı ve varsayılan değer kullanılmamalı (yerelde APP_ENV=development ayarlayın)", minJWTSecretLength)
	}
	return []byte(secret), nil
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// Güvenli rastgele sayı üretici [0, max)
func secureRandInt(max int) int {
	n, err := rand.Int(rand.Reader, big.NewInt(int64(max)))
	if err != nil {
		return int(time.Now().UnixNano() % int64(max))
	}
	return int(n.Int64())
}

func randomDie() int {
	return secureRandInt(6) + 1
}

var (
	liveMu      sync.Mutex // Yaprak kilit: tutulurken başka kilit alınmaz
	liveClients = make(map[int]map[*Client]struct{})
	activeGames = make(map[int]*Room)
)

func registerLiveClient(c *Client) {
	liveMu.Lock()
	defer liveMu.Unlock()
	if liveClients[c.UserID] == nil {
		liveClients[c.UserID] = make(map[*Client]struct{})
	}
	liveClients[c.UserID][c] = struct{}{}
}

func unregisterLiveClient(c *Client) {
	liveMu.Lock()
	defer liveMu.Unlock()
	delete(liveClients[c.UserID], c)
	if len(liveClients[c.UserID]) == 0 {
		delete(liveClients, c.UserID)
	}
}

// Kapatılan veya silinen hesabın açık bağlantılarını keser
func disconnectUser(userID int) {
	liveMu.Lock()
	clients := make([]*Client, 0, len(liveClients[userID]))
	for c := range liveClients[userID] {
		clients = append(clients, c)
	}
	liveMu.Unlock()
	for _, c := range clients {
		_ = c.Conn.Close()
	}
}

func setActiveGame(userID int, room *Room) {
	if userID <= 0 {
		return
	}
	liveMu.Lock()
	activeGames[userID] = room
	liveMu.Unlock()
}

func activeGameFor(userID int) *Room {
	liveMu.Lock()
	defer liveMu.Unlock()
	return activeGames[userID]
}

func clearActiveGame(r *Room) {
	liveMu.Lock()
	defer liveMu.Unlock()
	for _, p := range []*Client{r.Player1, r.Player2} {
		if p != nil && activeGames[p.UserID] == r {
			delete(activeGames, p.UserID)
		}
	}
}

// Kuyrukta, bu oyuncuyla eşleşebilecek ilk rakibi bulur (mu tutulurken çağrılır)
func takeWaitingOpponent(client *Client) *Client {
	for i, candidate := range waitingClients {
		if candidate == client || candidate.UserID == client.UserID || isBlockedEither(candidate.UserID, client.UserID) {
			continue
		}
		waitingClients = append(waitingClients[:i:i], waitingClients[i+1:]...)
		return candidate
	}
	return nil
}

func isUserQueued(userID int) bool {
	for _, candidate := range waitingClients {
		if candidate.UserID == userID {
			return true
		}
	}
	return false
}

func removeFromQueue(client *Client) {
	for i, candidate := range waitingClients {
		if candidate == client {
			waitingClients = append(waitingClients[:i:i], waitingClients[i+1:]...)
			return
		}
	}
}

type Credentials struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Email    string `json:"email"`

	AcceptedTerms bool `json:"acceptedTerms"`
}

func loadDotEnv(paths ...string) {
	for _, p := range paths {
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		for _, line := range strings.Split(string(data), "\n") {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			parts := strings.SplitN(line, "=", 2)
			if len(parts) != 2 {
				continue
			}
			k := strings.TrimSpace(parts[0])
			v := strings.Trim(strings.TrimSpace(parts[1]), `"'`)
			if k != "" && os.Getenv(k) == "" {
				_ = os.Setenv(k, v)
			}
		}
	}
}

func initDB() {
	loadDotEnv(".env", "../.env")
	dev := isDevelopment()
	var err error
	jwtKey, err = resolveJWTSecret(os.Getenv("JWT_SECRET"), dev)
	if err != nil {
		log.Fatal("Güvenli olmayan yapılandırma: ", err)
	}
	connStr := os.Getenv("DATABASE_URL")
	if connStr == "" {
		if !dev {
			log.Fatal("Güvenli olmayan yapılandırma: üretimde DATABASE_URL zorunludur (yerelde APP_ENV=development ayarlayın)")
		}
		connStr = "host=localhost port=5432 user=yatzy_user password=yatzy_password dbname=yatzy_gamedb sslmode=disable"
	}

	db, err = sql.Open("postgres", connStr)
	if err != nil {
		log.Fatal("Veritabanı bağlantı hatası:", err)
	}

	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(5 * time.Minute)

	err = db.Ping()
	if err != nil {
		log.Fatal("Veritabanına ulaşılamıyor (Docker çalışıyor mu?):", err)
	}

	fmt.Println("✅ PostgreSQL veritabanına başarıyla bağlanıldı!")
	createTables()
}

// 5 Haneli Çakışmasız (Collision-Free) Rastgele Oda Kodu Üretici
func generateUniqueRoomCode() string {
	const charset = "ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	for {
		b := make([]byte, 5)
		for i := range b {
			b[i] = charset[secureRandInt(len(charset))]
		}
		code := string(b)
		if _, exists := privateRooms[code]; !exists {
			return code
		}
	}
}

// Süresi dolmuş özel odaları temizleyen arka plan işçisi (Memory Leak Önleyici)
func startRoomCleanupWorker() {
	ticker := time.NewTicker(2 * time.Minute)
	go func() {
		for range ticker.C {
			mu.Lock()
			now := time.Now()
			for code, entry := range privateRooms {
				if now.Sub(entry.CreatedAt) > RoomExpiryMinutes*time.Minute {
					delete(privateRooms, code)
					if entry.Host != nil {
						_ = entry.Host.WriteJSON(map[string]interface{}{
							"type":    "ERROR",
							"message": "Özel oda bekleme süresi dolduğu için kapatıldı.",
						})
					}
				}
			}
			mu.Unlock()
			pruneChatRecords()
		}
	}()
}

// Sunucu Taraflı (Server-Authoritative) Puan Hesaplayıcı
func calcScoreServer(key string, dice [5]int) int {
	sum := 0
	cnt := make(map[int]int)
	for _, d := range dice {
		sum += d
		cnt[d]++
	}

	switch key {
	case "ones":
		return cnt[1] * 1
	case "twos":
		return cnt[2] * 2
	case "threes":
		return cnt[3] * 3
	case "fours":
		return cnt[4] * 4
	case "fives":
		return cnt[5] * 5
	case "sixes":
		return cnt[6] * 6
	case "chance":
		return sum
	case "three":
		for n := 1; n <= 6; n++ {
			if cnt[n] >= 3 {
				return sum
			}
		}
		return 0
	case "four":
		for n := 1; n <= 6; n++ {
			if cnt[n] >= 4 {
				return sum
			}
		}
		return 0
	case "full":
		h3, h2 := false, false
		for n := 1; n <= 6; n++ {
			if cnt[n] == 3 {
				h3 = true
			}
			if cnt[n] == 2 {
				h2 = true
			}
			if cnt[n] == 5 { // 5 aynı zar da kurallara göre Full Ev sayılabilir veya 3+2
				h3, h2 = true, true
			}
		}
		if h3 && h2 {
			return 25
		}
		return 0
	case "small":
		has := func(a, b, c, d int) bool {
			return cnt[a] > 0 && cnt[b] > 0 && cnt[c] > 0 && cnt[d] > 0
		}
		if has(1, 2, 3, 4) || has(2, 3, 4, 5) || has(3, 4, 5, 6) {
			return 30
		}
		return 0
	case "large":
		uniq := make([]int, 0, len(cnt))
		for k := range cnt {
			uniq = append(uniq, k)
		}
		if len(uniq) < 5 {
			return 0
		}
		sort.Ints(uniq)
		if (uniq[0] == 1 && uniq[4] == 5) || (uniq[0] == 2 && uniq[4] == 6) {
			return 40
		}
		return 0
	case "yacht":
		for n := 1; n <= 6; n++ {
			if cnt[n] == 5 {
				return 50
			}
		}
		return 0
	}
	return 0
}

func computeTotalAndBonus(scores map[string]int) (total int, upper int, bonus int) {
	for _, k := range upperCategories {
		if v, ok := scores[k]; ok {
			upper += v
		}
	}
	if upper >= 63 {
		bonus = 35
	}
	for _, v := range scores {
		total += v
	}
	total += bonus
	return total, upper, bonus
}

// Kullanıcının seçilen bahis için yeterli jetonu olup olmadığını kontrol eder
func hasEnoughTokensDB(userID int, bet int) bool {
	if bet <= 0 || userID <= 0 {
		return true
	}
	var tokens int
	err := db.QueryRow(`SELECT tokens FROM profiles WHERE user_id = $1`, userID).Scan(&tokens)
	return err == nil && tokens >= bet
}

// Bahis miktarını veritabanından atomik olarak düşer (yeterli bakiyesi yoksa false döner)
func deductBetDB(userID int, bet int) bool {
	if bet <= 0 || userID <= 0 {
		return true
	}
	res, err := db.Exec(`UPDATE profiles SET tokens = tokens - $1 WHERE user_id = $2 AND tokens >= $1`, bet, userID)
	if err != nil {
		return false
	}
	rows, _ := res.RowsAffected()
	return rows > 0
}

// İki oyuncudan da bahis miktarını güvenli şekilde düşer; ikinci oyuncuda bakiye yetmezse birinciye anında iade eder
func deductBothBetsDB(user1ID int, user2ID int, bet int) bool {
	if bet <= 0 {
		return true
	}
	if !deductBetDB(user1ID, bet) {
		return false
	}
	if !deductBetDB(user2ID, bet) {
		if user1ID > 0 {
			_, _ = db.Exec(`UPDATE profiles SET tokens = tokens + $1 WHERE user_id = $2`, bet, user1ID)
		}
		return false
	}
	return true
}

// Veritabanında maç sonu ödüllerini ve görev istatistiklerini güvenli şekilde günceller
func awardMatchResultDB(userID int, won bool, score int, xpGain int, tokenGain int, caseGain int, upperBonusWon bool, yachtsInMatch int) {
	if userID <= 0 {
		return
	}
	winInc := 0
	if won {
		winInc = 1
	}
	bonusInc := 0
	if upperBonusWon {
		bonusInc = 1
	}
	_, err := db.Exec(`
		UPDATE profiles
		SET xp = xp + $1,
		    tokens = tokens + $2,
		    cases = cases + $3,
		    wins = wins + $4,
		    games_played = games_played + 1,
		    high_score = GREATEST(high_score, $5),
		    upper_bonuses_won = COALESCE(upper_bonuses_won, 0) + $6,
		    total_yachts = COALESCE(total_yachts, 0) + $7,
		    max_yachts_in_match = GREATEST(COALESCE(max_yachts_in_match, 0), $7)
		WHERE user_id = $8
	`, xpGain, tokenGain, caseGain, winInc, score, bonusInc, yachtsInMatch, userID)
	if err != nil {
		log.Println("Maç ödülü DB güncelleme hatası:", err)
	}
}

func newRoom(id string, p1, p2 *Client) *Room {
	if p1 != nil {
		setUserGameState(p1.UserID, true)
	}
	if p2 != nil {
		setUserGameState(p2.UserID, true)
	}
	r := &Room{
		ID:        id,
		Player1:   p1,
		Player2:   p2,
		Turn:      "p1",
		Round:     1,
		RollsLeft: 3,
		Dice:      [5]int{3, 1, 5, 2, 4},
		Locked:    [5]bool{false, false, false, false, false},
		P1Scores:  make(map[string]int),
		P2Scores:  make(map[string]int),
	}
	if p1 != nil {
		setActiveGame(p1.UserID, r)
	}
	if p2 != nil {
		setActiveGame(p2.UserID, r)
	}
	r.resetTurnTimerLocked()
	return r
}

func (r *Room) opponentOf(client *Client) *Client {
	if r.Player1 == client {
		return r.Player2
	}
	return r.Player1
}

// Maçı hükmen bitirir (r.Mu tutulurken çağrılır)
func (r *Room) forfeitLocked(loser *Client) {
	r.GameOver = true
	r.stopTurnTimerLocked()
	clearActiveGame(r)

	opponent := r.opponentOf(loser)
	if opponent == nil {
		return
	}
	forfeitTokens := 50 + (r.BetAmount * 2)
	awardMatchResultDB(opponent.UserID, true, 0, 150, forfeitTokens, 0, false, 0)
	_ = opponent.WriteJSON(map[string]interface{}{
		"type": "OPPONENT_LEFT",
		"reward": map[string]int{
			"xp":     150,
			"tokens": forfeitTokens,
		},
	})
	opponent.Room = nil
	if !opponent.Disconnected {
		setUserGameState(opponent.UserID, false)
	}
}

// Bağlantısı kopan oyuncuya geri dönmesi için süre tanır
func (r *Room) handleDisconnect(client *Client) {
	r.Mu.Lock()
	defer r.Mu.Unlock()
	setUserGameState(client.UserID, false)
	if r.GameOver || (r.Player1 != client && r.Player2 != client) {
		return
	}
	client.Disconnected = true
	if opponent := r.opponentOf(client); opponent != nil {
		_ = opponent.WriteJSON(map[string]interface{}{
			"type":    "OPPONENT_DISCONNECTED",
			"seconds": int(ReconnectGrace / time.Second),
		})
	}
	go func() {
		time.Sleep(ReconnectGrace)
		r.Mu.Lock()
		defer r.Mu.Unlock()
		if !r.GameOver && (r.Player1 == client || r.Player2 == client) {
			r.forfeitLocked(client)
		}
	}()
}

// Oyuncunun devam eden maçına yeni bağlantıyı bağlar (mu tutulurken çağrılır)
func tryResumeGame(client *Client) bool {
	room := activeGameFor(client.UserID)
	if room == nil {
		return false
	}
	room.Mu.Lock()
	defer room.Mu.Unlock()
	if room.GameOver {
		return false
	}

	slot := &room.Player1
	playerIndex := 1
	if room.Player2 != nil && room.Player2.UserID == client.UserID {
		slot = &room.Player2
		playerIndex = 2
	}
	previous := *slot
	if previous == nil || previous.UserID != client.UserID {
		return false
	}
	*slot = client
	client.Room = room
	client.BetAmount = room.BetAmount
	if previous.Disconnected {
		setUserGameState(client.UserID, true)
	} else {
		previous.Room = nil
		_ = previous.WriteJSON(map[string]interface{}{"type": "SESSION_REPLACED"})
		go func() { _ = previous.Conn.Close() }()
	}

	opponent := room.opponentOf(client)
	opponentName := "🤖 YatBot"
	if opponent != nil {
		opponentName = opponent.Username
		_ = opponent.WriteJSON(map[string]interface{}{"type": "OPPONENT_RECONNECTED"})
	}
	p1Total, p1Upper, p1Bonus := computeTotalAndBonus(room.P1Scores)
	p2Total, p2Upper, p2Bonus := computeTotalAndBonus(room.P2Scores)
	secondsLeft := int(time.Until(room.TurnDeadline).Seconds())
	if secondsLeft < 0 {
		secondsLeft = 0
	}
	_ = client.WriteJSON(map[string]interface{}{
		"type":         "MATCH_FOUND",
		"resumed":      true,
		"room":         room.ID,
		"playerIndex":  playerIndex,
		"opponentName": opponentName,
		"turnSeconds":  TurnDurationSeconds,
		"betAmount":    room.BetAmount,
		"state": map[string]interface{}{
			"dices":       room.Dice,
			"locked":      room.Locked,
			"rollsLeft":   room.RollsLeft,
			"turn":        room.Turn,
			"round":       room.Round,
			"p1Scores":    room.P1Scores,
			"p2Scores":    room.P2Scores,
			"p1Total":     p1Total,
			"p2Total":     p2Total,
			"p1Upper":     p1Upper,
			"p2Upper":     p2Upper,
			"p1Bonus":     p1Bonus,
			"p2Bonus":     p2Bonus,
			"secondsLeft": secondsLeft,
		},
	})
	return true
}

func (r *Room) stopTurnTimerLocked() {
	if r.TurnTimerStop != nil {
		close(r.TurnTimerStop)
		r.TurnTimerStop = nil
	}
}

// Her tur için 30 saniyelik süre başlatır. Süre dolarsa otomatik zar atıp en uygun kategoriyi seçer (AFK Koruması)
func (r *Room) resetTurnTimerLocked() {
	r.stopTurnTimerLocked()
	if r.GameOver {
		return
	}
	stopCh := make(chan struct{})
	r.TurnTimerStop = stopCh
	r.TurnDeadline = time.Now().Add(TurnDurationSeconds * time.Second)

	go func(expectedRound int, expectedTurn string, stop <-chan struct{}) {
		timer := time.NewTimer(TurnDurationSeconds * time.Second)
		defer timer.Stop()
		select {
		case <-stop:
			return
		case <-timer.C:
			r.Mu.Lock()
			defer r.Mu.Unlock()
			if r.GameOver || r.Round != expectedRound || r.Turn != expectedTurn {
				return
			}
			// Oyuncu hiç zar atmadıysa önce otomatik zar at
			if r.RollsLeft == 3 {
				for i := 0; i < 5; i++ {
					r.Dice[i] = randomDie()
				}
				r.RollsLeft = 2
			}
			// Açık olan ilk kategoriyi (veya en yüksek puan getireni) otomatik seç
			scores := r.P1Scores
			if r.Turn == "p2" {
				scores = r.P2Scores
			}
			bestKey := ""
			bestPts := -1
			for k := range allowedCategories {
				if _, used := scores[k]; !used {
					pts := calcScoreServer(k, r.Dice)
					if pts > bestPts {
						bestPts = pts
						bestKey = k
					}
				}
			}
			if bestKey != "" {
				r.applyScoreLocked(bestKey, true)
			}
		}
	}(r.Round, r.Turn, stopCh)
}

func (r *Room) broadcastLocked(msg map[string]interface{}) {
	p1, p2 := r.Player1, r.Player2
	if p1 != nil {
		_ = p1.WriteJSON(msg)
	}
	if p2 != nil {
		_ = p2.WriteJSON(msg)
	}
}

func (r *Room) applyScoreLocked(key string, autoSelected bool) {
	scores := r.P1Scores
	if r.Turn == "p2" {
		scores = r.P2Scores
	}
	if _, already := scores[key]; already {
		return
	}

	pts := calcScoreServer(key, r.Dice)
	scores[key] = pts
	scoredPlayer := r.Turn

	// Eğer bu turda zarların 5'i de aynıysa (YAT atıldıysa), oyuncunun maç içi YAT sayacını artır
	if calcScoreServer("yacht", r.Dice) == 50 {
		if scoredPlayer == "p1" {
			r.P1YachtCount++
		} else {
			r.P2YachtCount++
		}
	}

	if r.Turn == "p1" {
		r.Turn = "p2"
	} else {
		r.Turn = "p1"
		if r.Round < 13 {
			r.Round++
		}
	}

	r.RollsLeft = 3
	r.Locked = [5]bool{false, false, false, false, false}
	r.Dice = [5]int{3, 1, 5, 2, 4}

	p1Total, p1Upper, p1Bonus := computeTotalAndBonus(r.P1Scores)
	p2Total, p2Upper, p2Bonus := computeTotalAndBonus(r.P2Scores)

	isGameOver := len(r.P1Scores) >= 13 && len(r.P2Scores) >= 13
	if isGameOver {
		r.GameOver = true
		r.stopTurnTimerLocked()
		clearActiveGame(r)
	} else {
		r.resetTurnTimerLocked()
		if r.IsBotMatch && r.Turn == "p2" {
			go r.runBotTurn(r.Round)
		}
	}

	msg := map[string]interface{}{
		"type":         "SCORE_SELECTED",
		"key":          key,
		"pts":          pts,
		"player":       scoredPlayer,
		"nextTurn":     r.Turn,
		"round":        r.Round,
		"autoSelected": autoSelected,
		"p1Scores":     r.P1Scores,
		"p2Scores":     r.P2Scores,
		"p1Total":      p1Total,
		"p2Total":      p2Total,
		"p1Upper":      p1Upper,
		"p2Upper":      p2Upper,
		"p1Bonus":      p1Bonus,
		"p2Bonus":      p2Bonus,
		"turnSeconds":  TurnDurationSeconds,
		"gameOver":     isGameOver,
	}

	if isGameOver {
		isDraw := p1Total == p2Total
		p1Won := p1Total > p2Total
		p2Won := p2Total > p1Total

		p1XP, p1Tokens, p1Cases := 150, 40, 0
		p2XP, p2Tokens, p2Cases := 150, 40, 0

		if isDraw {
			// Beraberlik durumunda her iki oyuncuya da kendi bahsi (1x) iade edilir + 60 jeton verilir
			p1XP, p1Tokens, p1Cases = 220, 60+r.BetAmount, 0
			p2XP, p2Tokens, p2Cases = 220, 60+r.BetAmount, 0
		} else if p1Won {
			p1XP, p1Tokens, p1Cases = 300, 100+(r.BetAmount*2), 1
		} else if p2Won {
			p2XP, p2Tokens, p2Cases = 300, 100+(r.BetAmount*2), 1
		}

		if r.Player1 != nil {
			awardMatchResultDB(r.Player1.UserID, p1Won, p1Total, p1XP, p1Tokens, p1Cases, p1Bonus > 0, r.P1YachtCount)
		}
		if r.Player2 != nil {
			awardMatchResultDB(r.Player2.UserID, p2Won, p2Total, p2XP, p2Tokens, p2Cases, p2Bonus > 0, r.P2YachtCount)
		}

		msg["isDraw"] = isDraw
		msg["betAmount"] = r.BetAmount
		msg["p1Yachts"] = r.P1YachtCount
		msg["p2Yachts"] = r.P2YachtCount
		msg["rewards"] = map[string]interface{}{
			"p1": map[string]int{"xp": p1XP, "tokens": p1Tokens, "cases": p1Cases, "betPot": r.BetAmount * 2},
			"p2": map[string]int{"xp": p2XP, "tokens": p2Tokens, "cases": p2Cases, "betPot": r.BetAmount * 2},
		}
	}

	r.broadcastLocked(msg)
}

// Botun en uygun zarları kilitleme stratejisi
func (r *Room) chooseBotLocksLocked() [5]bool {
	var locks [5]bool
	cnt := make(map[int]int)
	for _, d := range r.Dice {
		cnt[d]++
	}
	// En çok tekrar eden zar değerini bul
	bestVal, maxCnt := 6, 0
	for v := 6; v >= 1; v-- {
		if cnt[v] > maxCnt {
			maxCnt = cnt[v]
			bestVal = v
		}
	}
	if maxCnt >= 2 {
		for i, d := range r.Dice {
			if d == bestVal {
				locks[i] = true
			}
		}
		return locks
	}
	// Eğer tekrar eden yoksa en yüksek değerli zarları (5 ve 6) tut
	for i, d := range r.Dice {
		if d >= 5 {
			locks[i] = true
		}
	}
	return locks
}

// Botun mevcut zarlarla en yüksek değerli kategoriyi seçme stratejisi
func (r *Room) chooseBotBestCategoryLocked() string {
	priorityOrder := []string{
		"yacht", "large", "small", "full", "four", "three",
		"sixes", "fives", "fours", "threes", "twos", "ones", "chance",
	}
	bestKey := ""
	bestWeight := -1000

	for _, k := range priorityOrder {
		if _, used := r.P2Scores[k]; used {
			continue
		}
		pts := calcScoreServer(k, r.Dice)
		weight := pts
		// Yüksek değerli kategorileri 0 puanla harcamaktan kaçın
		if pts == 0 {
			switch k {
			case "yacht":
				weight = -50
			case "large", "small", "full":
				weight = -30
			case "sixes", "fives", "four":
				weight = -20
			case "ones":
				weight = -2
			case "twos":
				weight = -4
			default:
				weight = -10
			}
		} else if k == "yacht" || k == "large" || k == "full" || k == "small" {
			weight += 15 // Özel kombinasyon yakalandıysa öncelik ver
		}

		if weight > bestWeight {
			bestWeight = weight
			bestKey = k
		}
	}
	if bestKey == "" {
		for k := range allowedCategories {
			if _, used := r.P2Scores[k]; !used {
				return k
			}
		}
	}
	return bestKey
}

// Botun sırası geldiğinde gerçek bir oyuncu gibi zar atıp kilitleyen ve puan seçen goroutine
func (r *Room) runBotTurn(expectedRound int) {
	// 1. İlk zar atışı öncesi kısa bekleme
	time.Sleep(1300 * time.Millisecond)

	r.Mu.Lock()
	if r.GameOver || r.Turn != "p2" || r.Round != expectedRound {
		r.Mu.Unlock()
		return
	}
	r.Locked = [5]bool{false, false, false, false, false}
	for i := 0; i < 5; i++ {
		r.Dice[i] = randomDie()
	}
	r.RollsLeft = 2
	r.broadcastLocked(map[string]interface{}{
		"type":      "DICE_ROLLED",
		"dices":     r.Dice,
		"locked":    r.Locked,
		"rollsLeft": r.RollsLeft,
	})
	// Eğer ilk atışta YAT, Büyük Sıralı veya Full Ev geldiyse direkt puanı yaz
	if calcScoreServer("yacht", r.Dice) == 50 || calcScoreServer("large", r.Dice) == 40 || calcScoreServer("full", r.Dice) == 25 {
		bestKey := r.chooseBotBestCategoryLocked()
		r.Mu.Unlock()
		time.Sleep(1400 * time.Millisecond)
		r.Mu.Lock()
		if !r.GameOver && r.Turn == "p2" && r.Round == expectedRound {
			r.applyScoreLocked(bestKey, false)
		}
		r.Mu.Unlock()
		return
	}
	locks1 := r.chooseBotLocksLocked()
	r.Mu.Unlock()

	// 2. Zarları kilitleme animasyonu
	time.Sleep(1200 * time.Millisecond)
	r.Mu.Lock()
	if r.GameOver || r.Turn != "p2" || r.Round != expectedRound {
		r.Mu.Unlock()
		return
	}
	r.Locked = locks1
	r.broadcastLocked(map[string]interface{}{
		"type":   "TOGGLE_LOCK",
		"locked": r.Locked,
	})
	r.Mu.Unlock()

	// 3. İkinci zar atışı
	time.Sleep(900 * time.Millisecond)
	r.Mu.Lock()
	if r.GameOver || r.Turn != "p2" || r.Round != expectedRound {
		r.Mu.Unlock()
		return
	}
	for i := 0; i < 5; i++ {
		if !r.Locked[i] {
			r.Dice[i] = randomDie()
		}
	}
	r.RollsLeft = 1
	r.broadcastLocked(map[string]interface{}{
		"type":      "DICE_ROLLED",
		"dices":     r.Dice,
		"locked":    r.Locked,
		"rollsLeft": r.RollsLeft,
	})
	locks2 := r.chooseBotLocksLocked()
	r.Mu.Unlock()

	// 4. İkinci kilitleme ve üçüncü atış (eğer tüm zarlar kilitli değilse)
	allLocked := true
	for _, b := range locks2 {
		if !b {
			allLocked = false
			break
		}
	}

	if !allLocked {
		time.Sleep(1100 * time.Millisecond)
		r.Mu.Lock()
		if r.GameOver || r.Turn != "p2" || r.Round != expectedRound {
			r.Mu.Unlock()
			return
		}
		r.Locked = locks2
		r.broadcastLocked(map[string]interface{}{
			"type":   "TOGGLE_LOCK",
			"locked": r.Locked,
		})
		r.Mu.Unlock()

		time.Sleep(900 * time.Millisecond)
		r.Mu.Lock()
		if r.GameOver || r.Turn != "p2" || r.Round != expectedRound {
			r.Mu.Unlock()
			return
		}
		for i := 0; i < 5; i++ {
			if !r.Locked[i] {
				r.Dice[i] = randomDie()
			}
		}
		r.RollsLeft = 0
		r.broadcastLocked(map[string]interface{}{
			"type":      "DICE_ROLLED",
			"dices":     r.Dice,
			"locked":    r.Locked,
			"rollsLeft": r.RollsLeft,
		})
		r.Mu.Unlock()
	}

	// 5. En uygun kategoriyi seçip turu tamamla
	time.Sleep(1400 * time.Millisecond)
	r.Mu.Lock()
	if !r.GameOver && r.Turn == "p2" && r.Round == expectedRound {
		bestKey := r.chooseBotBestCategoryLocked()
		r.applyScoreLocked(bestKey, false)
	}
	r.Mu.Unlock()
}

// WebSocket kimliğini token ve veritabanından çözer; kullanıcı adı istemciden alınmaz
func authenticateWebSocket(r *http.Request) (int, string, error) {
	uidStr, err := parseTokenString(r.URL.Query().Get("token"))
	if err != nil {
		return 0, "", err
	}
	userID, err := strconv.Atoi(uidStr)
	if err != nil || userID <= 0 {
		return 0, "", fmt.Errorf("geçersiz kullanıcı")
	}
	var username string
	if err := db.QueryRow(`SELECT username FROM users WHERE id = $1`, userID).Scan(&username); err != nil {
		return 0, "", err
	}
	return userID, username, nil
}

// Bağlantı başına saniyelik mesaj sayacı (yalnızca o bağlantının okuma döngüsünden çağrılır)
type wsMessageBudget struct {
	windowStart time.Time
	count       int
}

func (b *wsMessageBudget) allow(now time.Time) bool {
	if now.Sub(b.windowStart) >= time.Second {
		b.windowStart = now
		b.count = 0
	}
	b.count++
	return b.count <= maxWSMessagesPerSecond
}

func handleConnections(w http.ResponseWriter, r *http.Request) {
	if !generalLimiter.Allow(getClientIP(r)) {
		http.Error(w, "Çok fazla istek gönderdiniz. Lütfen kısa bir süre bekleyin.", http.StatusTooManyRequests)
		return
	}

	userID, username, err := authenticateWebSocket(r)
	if err != nil {
		http.Error(w, "Yetkisiz", http.StatusUnauthorized)
		return
	}

	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println("WebSocket hatası:", err)
		return
	}

	client := &Client{Conn: ws, UserID: userID, Username: username}
	registerLiveClient(client)

	// Ping/Pong ve ReadDeadline ayarları (Hayalet bağlantıları tespit eder)
	ws.SetReadLimit(8192)
	_ = ws.SetReadDeadline(time.Now().Add(PongWait))
	ws.SetPongHandler(func(string) error {
		_ = ws.SetReadDeadline(time.Now().Add(PongWait))
		return nil
	})

	// Periyodik Ping gönderici
	done := make(chan struct{})
	go func() {
		ticker := time.NewTicker(PingPeriod)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				client.WriteMu.Lock()
				_ = ws.SetWriteDeadline(time.Now().Add(10 * time.Second))
				err := ws.WriteMessage(websocket.PingMessage, nil)
				client.WriteMu.Unlock()
				if err != nil {
					return
				}
			}
		}
	}()

	defer func() {
		close(done)
		ws.Close()

		unregisterLiveClient(client)
		mu.Lock()
		removeFromQueue(client)
		for code, entry := range privateRooms {
			if entry.Host == client {
				delete(privateRooms, code)
			}
		}
		room := client.Room
		client.Room = nil
		mu.Unlock()
		if room != nil {
			room.handleDisconnect(client)
		}
	}()

	var budget wsMessageBudget
	for {
		var msg map[string]interface{}
		err := ws.ReadJSON(&msg)
		if err != nil {
			break
		}
		_ = ws.SetReadDeadline(time.Now().Add(PongWait))

		if !budget.allow(time.Now()) {
			log.Printf("WebSocket mesaj sınırı aşıldı, bağlantı kapatılıyor (kullanıcı %d)", client.UserID)
			break
		}

		// 1. EŞLEŞTİRME VE ODA KURMA AKSİYONLARI
		if actionRaw, hasAction := msg["action"]; hasAction {
			action, ok := actionRaw.(string)
			if !ok {
				continue
			}

			mu.Lock()
			// Eğer oyuncunun zaten odası varsa tekrar kuyruğa girmesini engelle
			if client.Room != nil {
				mu.Unlock()
				continue
			}

			// Devam eden maçı olan oyuncu yeni maça değil, o maça döner
			if tryResumeGame(client) {
				mu.Unlock()
				continue
			}
			if action == "RESUME" {
				_ = client.WriteJSON(map[string]interface{}{"type": "RESUME_FAILED"})
				mu.Unlock()
				continue
			}

			requestedBet := normalizeBet(msg["betAmount"])
			client.BetAmount = requestedBet

			if action == "BOT_MATCH" {
				if !deductBetDB(client.UserID, requestedBet) {
					_ = client.WriteJSON(map[string]interface{}{"type": "ERROR", "message": "Seçilen masa için yeterli jetonunuz yok!"})
					mu.Unlock()
					continue
				}
				// Bota karşı anında tek oyunculu maç başlat
				removeFromQueue(client)
				roomCounter++
				roomID := fmt.Sprintf("BOT-ROOM-%d", roomCounter)
				room := newRoom(roomID, client, nil)
				room.IsBotMatch = true
				room.BetAmount = requestedBet
				client.Room = room

				_ = client.WriteJSON(map[string]interface{}{
					"type":         "MATCH_FOUND",
					"room":         roomID,
					"playerIndex":  1,
					"opponentName": "🤖 YatBot",
					"turnSeconds":  TurnDurationSeconds,
					"betAmount":    requestedBet,
				})

			} else if action == "QUICK_MATCH" {
				if !hasEnoughTokensDB(client.UserID, requestedBet) {
					requestedBet = 0
					client.BetAmount = 0
				}
				removeFromQueue(client)
				// Aynı hesabın ikinci bağlantıdan kuyruğa girmesini önle
				if isUserQueued(client.UserID) {
					_ = client.WriteJSON(map[string]interface{}{"type": "ERROR", "message": "Bu hesap zaten eşleşme kuyruğunda!"})
					mu.Unlock()
					continue
				}
				opponent := takeWaitingOpponent(client)
				if opponent == nil {
					waitingClients = append(waitingClients, client)
					_ = client.WriteJSON(map[string]interface{}{"type": "WAITING_FOR_OPPONENT"})
				} else {
					matchBet := opponent.BetAmount
					if requestedBet < matchBet {
						matchBet = requestedBet
					}
					if !deductBothBetsDB(opponent.UserID, client.UserID, matchBet) {
						matchBet = 0
					}

					roomCounter++
					roomID := fmt.Sprintf("ROOM-%d", roomCounter)
					room := newRoom(roomID, opponent, client)
					room.BetAmount = matchBet
					opponent.Room = room
					client.Room = room

					_ = opponent.WriteJSON(map[string]interface{}{
						"type":         "MATCH_FOUND",
						"room":         roomID,
						"playerIndex":  1,
						"opponentName": client.Username,
						"turnSeconds":  TurnDurationSeconds,
						"betAmount":    matchBet,
					})
					_ = client.WriteJSON(map[string]interface{}{
						"type":         "MATCH_FOUND",
						"room":         roomID,
						"playerIndex":  2,
						"opponentName": opponent.Username,
						"turnSeconds":  TurnDurationSeconds,
						"betAmount":    matchBet,
					})
				}

			} else if action == "CREATE_ROOM" {
				if !hasEnoughTokensDB(client.UserID, requestedBet) {
					requestedBet = 0
					client.BetAmount = 0
				}
				// Önceden açtığı özel oda varsa temizle (Duplicate room leak önleyici)
				for code, entry := range privateRooms {
					if entry.Host == client {
						delete(privateRooms, code)
					}
				}
				code := generateUniqueRoomCode()
				privateRooms[code] = PrivateRoomEntry{Host: client, BetAmount: requestedBet, CreatedAt: time.Now()}
				_ = client.WriteJSON(map[string]interface{}{"type": "ROOM_CREATED", "roomCode": code, "betAmount": requestedBet})

			} else if action == "JOIN_ROOM" {
				// Güvenli tip dönüşümü (Panic önleyici)
				codeRaw, ok := msg["roomCode"].(string)
				if !ok {
					_ = client.WriteJSON(map[string]interface{}{"type": "ERROR", "message": "Geçersiz oda kodu formatı!"})
					mu.Unlock()
					continue
				}
				code := strings.ToUpper(strings.TrimSpace(codeRaw))
				entry, exists := privateRooms[code]

				if exists && entry.Host != nil && entry.Host != client && entry.Host.UserID != client.UserID && !isBlockedEither(entry.Host.UserID, client.UserID) {
					matchBet := entry.BetAmount
					if !deductBothBetsDB(entry.Host.UserID, client.UserID, matchBet) {
						matchBet = 0
					}
					delete(privateRooms, code)
					host := entry.Host

					roomCounter++
					roomID := fmt.Sprintf("ROOM-%d", roomCounter)
					room := newRoom(roomID, host, client)
					room.BetAmount = matchBet
					host.Room = room
					client.Room = room

					_ = host.WriteJSON(map[string]interface{}{
						"type":         "MATCH_FOUND",
						"room":         roomID,
						"playerIndex":  1,
						"opponentName": client.Username,
						"turnSeconds":  TurnDurationSeconds,
						"betAmount":    matchBet,
					})
					_ = client.WriteJSON(map[string]interface{}{
						"type":         "MATCH_FOUND",
						"room":         roomID,
						"playerIndex":  2,
						"opponentName": host.Username,
						"turnSeconds":  TurnDurationSeconds,
						"betAmount":    matchBet,
					})
				} else {
					_ = client.WriteJSON(map[string]interface{}{"type": "ERROR", "message": "Oda bulunamadı veya kendi odanıza katılamazsınız!"})
				}
			}
			mu.Unlock()
			continue
		}

		// 2. OYUN İÇİ MESAJLAŞMALAR (Server-Authoritative)
		room := client.Room
		if room == nil {
			continue
		}

		msgType, _ := msg["type"].(string)

		room.Mu.Lock()
		if room.GameOver {
			room.Mu.Unlock()
			continue
		}

		playerRole := "p1"
		if room.Player2 == client {
			playerRole = "p2"
		}

		// Maç sırasında engellenen oyuncudan mesaj iletilmez; gönderen yalnızca kendi mesajını görür
		deliverSocial := func(payload map[string]interface{}) {
			if opponent := room.opponentOf(client); opponent != nil && isBlockedEither(client.UserID, opponent.UserID) {
				_ = client.WriteJSON(payload)
				return
			}
			room.broadcastLocked(payload)
		}

		switch msgType {
		case "LEAVE":
			room.forfeitLocked(client)

		case "EMOJI":
			// Hızlı Emoji / Reaksiyon gönderimi (Anti-spam: 2 saniyede en fazla 1 emoji)
			if time.Since(client.LastEmojiAt) < 2000*time.Millisecond {
				_ = client.WriteJSON(map[string]interface{}{
					"type":    "SPAM_WARN",
					"message": "Emoji spam koruması: Lütfen 2 saniye bekleyin.",
				})
				room.Mu.Unlock()
				continue
			}
			emoji, ok := msg["emoji"].(string)
			if ok && len(emoji) > 0 && len(emoji) <= 16 {
				client.LastEmojiAt = time.Now()
				deliverSocial(map[string]interface{}{
					"type":   "EMOJI",
					"emoji":  emoji,
					"sender": playerRole,
				})
			}

		case "CHAT":
			// Oyun içi canlı sohbet (Anti-spam: 2.5 saniye bekleme süresi + 120 karakter sınırı)
			if time.Since(client.LastChatAt) < 2500*time.Millisecond {
				_ = client.WriteJSON(map[string]interface{}{
					"type":    "SPAM_WARN",
					"message": "Çok hızlı yazıyorsunuz! Lütfen 2.5 saniye bekleyin.",
				})
				room.Mu.Unlock()
				continue
			}
			rawText, _ := msg["text"].(string)
			cleanText := sanitizeChatMessage(rawText)
			if cleanText == "" {
				room.Mu.Unlock()
				continue
			}
			client.LastChatAt = time.Now()
			recordChatMessage(client.UserID, strings.TrimSpace(rawText))
			nowStr := time.Now().Format("15:04")
			deliverSocial(map[string]interface{}{
				"type":       "CHAT",
				"sender":     playerRole,
				"senderName": client.Username,
				"text":       cleanText,
				"time":       nowStr,
			})

			// Bota karşı oynanıyorsa YatBot samimi bir yanıt verir
			if room.IsBotMatch {
				go func(r *Room, userText string) {
					time.Sleep(1100 * time.Millisecond)
					r.Mu.Lock()
					defer r.Mu.Unlock()
					if r.GameOver {
						return
					}
					replies := []string{
						"Bol şans! Zarlar bugün güzel dönüyor 🎲",
						"İyi hamle! Bakalım YAT kimden gelecek 🔥",
						"Güzel maç oluyor, başarılar! 😎",
						"Hesaplamalarıma göre bu tur çok kritik 🤖",
						"Harika gidiyorsun! 👏",
					}
					botReply := replies[secureRandInt(len(replies))]
					r.broadcastLocked(map[string]interface{}{
						"type":       "CHAT",
						"sender":     "p2",
						"senderName": "🤖 YatBot",
						"text":       botReply,
						"time":       time.Now().Format("15:04"),
					})
				}(room, cleanText)
			}

		case "TOGGLE_LOCK":
			// Sadece sırası olan ve en az 1 kez zar atmış oyuncu zar kilitleyebilir
			if room.Turn != playerRole || room.RollsLeft == 3 {
				room.Mu.Unlock()
				continue
			}
			if lockedSlice, ok := msg["locked"].([]interface{}); ok && len(lockedSlice) == 5 {
				for i := 0; i < 5; i++ {
					if b, ok := lockedSlice[i].(bool); ok {
						room.Locked[i] = b
					}
				}
				room.broadcastLocked(map[string]interface{}{
					"type":   "TOGGLE_LOCK",
					"locked": room.Locked,
				})
			}

		case "ROLL_DICE":
			// Sadece sırası olan ve atış hakkı kalan oyuncu zar atabilir
			if room.Turn != playerRole || room.RollsLeft <= 0 {
				room.Mu.Unlock()
				continue
			}

			// İstemciden gelen güncel kilit durumunu uygula (eğer ilk atış değilse)
			if room.RollsLeft < 3 {
				if lockedSlice, ok := msg["locked"].([]interface{}); ok && len(lockedSlice) == 5 {
					for i := 0; i < 5; i++ {
						if b, ok := lockedSlice[i].(bool); ok {
							room.Locked[i] = b
						}
					}
				}
			} else {
				room.Locked = [5]bool{false, false, false, false, false}
			}

			// Sadece kilitli OLMAYAN zarları sunucuda yeniden üret
			for i := 0; i < 5; i++ {
				if !room.Locked[i] {
					room.Dice[i] = randomDie()
				}
			}
			room.RollsLeft--

			room.broadcastLocked(map[string]interface{}{
				"type":      "DICE_ROLLED",
				"dices":     room.Dice,
				"locked":    room.Locked,
				"rollsLeft": room.RollsLeft,
			})

		case "SCORE_SELECTED":
			// Sadece sırası olan ve en az 1 kez zar atmış oyuncu puan seçebilir
			if room.Turn != playerRole || room.RollsLeft == 3 {
				room.Mu.Unlock()
				continue
			}
			key, ok := msg["key"].(string)
			if !ok || !allowedCategories[key] {
				room.Mu.Unlock()
				continue
			}
			room.applyScoreLocked(key, false)
		}

		room.Mu.Unlock()
	}
}

func createTables() {
	usersTable := `
	CREATE TABLE IF NOT EXISTS users (
		id SERIAL PRIMARY KEY,
		username VARCHAR(50) UNIQUE NOT NULL,
		password_hash VARCHAR(255) NOT NULL,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	);`

	profilesTable := `
	CREATE TABLE IF NOT EXISTS profiles (
		user_id INT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
		xp INT DEFAULT 120,
		tokens INT DEFAULT 210,
		cases INT DEFAULT 2,
		wins INT DEFAULT 0,
		games_played INT DEFAULT 0,
		high_score INT DEFAULT 0,
		unlocked_dice TEXT DEFAULT 'ivory',
		selected_dice VARCHAR(20) DEFAULT 'ivory',
		login_streak INT DEFAULT 0,
		last_daily_reward VARCHAR(20) DEFAULT ''
	);`

	friendshipsTable := `
	CREATE TABLE IF NOT EXISTS friendships (
		id SERIAL PRIMARY KEY,
		requester_id INT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
		addressee_id INT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
		status VARCHAR(20) NOT NULL DEFAULT 'pending',
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		UNIQUE(requester_id, addressee_id)
	);`

	if _, err := db.Exec(usersTable); err != nil {
		log.Fatal("Users tablosu oluşturulamadı:", err)
	}
	if _, err := db.Exec(profilesTable); err != nil {
		log.Fatal("Profiles tablosu oluşturulamadı:", err)
	}
	if _, err := db.Exec(friendshipsTable); err != nil {
		log.Fatal("Friendships tablosu oluşturulamadı:", err)
	}

	// Mevcut veritabanı tabloları için eksik sütunları güvenli şekilde ekle (Migration)
	migrations := []string{
		`ALTER TABLE profiles ADD COLUMN IF NOT EXISTS unlocked_dice TEXT DEFAULT 'ivory';`,
		`ALTER TABLE profiles ADD COLUMN IF NOT EXISTS selected_dice VARCHAR(20) DEFAULT 'ivory';`,
		`ALTER TABLE profiles ADD COLUMN IF NOT EXISTS login_streak INT DEFAULT 0;`,
		`ALTER TABLE profiles ADD COLUMN IF NOT EXISTS last_daily_reward VARCHAR(20) DEFAULT '';`,
		`ALTER TABLE users ADD COLUMN IF NOT EXISTS email VARCHAR(255) DEFAULT '';`,
		`ALTER TABLE users ADD COLUMN IF NOT EXISTS google_id VARCHAR(255) DEFAULT '';`,
		`ALTER TABLE users ADD COLUMN IF NOT EXISTS guest_device_id VARCHAR(128) DEFAULT '';`,
		`ALTER TABLE users ADD COLUMN IF NOT EXISTS is_guest BOOLEAN DEFAULT FALSE;`,
		`ALTER TABLE profiles ADD COLUMN IF NOT EXISTS avatar TEXT DEFAULT '';`,
		`ALTER TABLE profiles ADD COLUMN IF NOT EXISTS country VARCHAR(10) DEFAULT 'TR';`,
		`ALTER TABLE profiles ADD COLUMN IF NOT EXISTS upper_bonuses_won INT DEFAULT 0;`,
		`ALTER TABLE profiles ADD COLUMN IF NOT EXISTS total_yachts INT DEFAULT 0;`,
		`ALTER TABLE profiles ADD COLUMN IF NOT EXISTS max_yachts_in_match INT DEFAULT 0;`,
		`ALTER TABLE profiles ADD COLUMN IF NOT EXISTS claimed_quests TEXT DEFAULT '';`,
		`CREATE TABLE IF NOT EXISTS user_reports (
			id SERIAL PRIMARY KEY,
			reporter_id INT DEFAULT 0,
			reported_username VARCHAR(60) NOT NULL,
			reason VARCHAR(255) NOT NULL,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		);`,
		`CREATE TABLE IF NOT EXISTS user_blocks (
			id SERIAL PRIMARY KEY,
			blocker_id INT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			blocked_username VARCHAR(60) NOT NULL,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			UNIQUE(blocker_id, blocked_username)
		);`,
		`ALTER TABLE users ADD COLUMN IF NOT EXISTS is_banned BOOLEAN DEFAULT FALSE;`,
		`ALTER TABLE users ADD COLUMN IF NOT EXISTS banned_reason VARCHAR(255) DEFAULT '';`,
		`ALTER TABLE users ADD COLUMN IF NOT EXISTS tokens_valid_after TIMESTAMPTZ;`,
		`ALTER TABLE users ADD COLUMN IF NOT EXISTS terms_accepted_at TIMESTAMPTZ;`,
		`ALTER TABLE user_blocks ADD COLUMN IF NOT EXISTS blocked_id INT REFERENCES users(id) ON DELETE CASCADE;`,
		`UPDATE user_blocks b SET blocked_id = u.id FROM users u WHERE b.blocked_id IS NULL AND LOWER(u.username) = LOWER(b.blocked_username);`,
		`DELETE FROM user_blocks WHERE blocked_id IS NULL;`,
		`CREATE INDEX IF NOT EXISTS user_blocks_pair ON user_blocks (blocker_id, blocked_id);`,
		`ALTER TABLE user_reports ADD COLUMN IF NOT EXISTS reported_user_id INT;`,
		`ALTER TABLE user_reports ADD COLUMN IF NOT EXISTS evidence TEXT DEFAULT '';`,
		`ALTER TABLE user_reports ADD COLUMN IF NOT EXISTS status VARCHAR(20) DEFAULT 'open';`,
		`ALTER TABLE user_reports ADD COLUMN IF NOT EXISTS resolved_at TIMESTAMPTZ;`,
		`CREATE TABLE IF NOT EXISTS deletion_requests (
			id SERIAL PRIMARY KEY,
			username VARCHAR(60) NOT NULL,
			email VARCHAR(255) DEFAULT '',
			status VARCHAR(20) DEFAULT 'open',
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		);`,
		`ALTER TABLE users ADD COLUMN IF NOT EXISTS apple_id VARCHAR(255) DEFAULT '';`,
		`ALTER TABLE users ADD COLUMN IF NOT EXISTS apple_refresh_token TEXT DEFAULT '';`,
		`CREATE UNIQUE INDEX IF NOT EXISTS users_apple_id_unique ON users (apple_id) WHERE apple_id <> '';`,
		`CREATE UNIQUE INDEX IF NOT EXISTS users_username_lower_unique ON users (LOWER(username));`,
		`CREATE UNIQUE INDEX IF NOT EXISTS users_google_id_unique ON users (google_id) WHERE google_id <> '';`,
	}
	for _, m := range migrations {
		if _, err := db.Exec(m); err != nil {
			log.Printf("⚠️ Şema güncellemesi uygulanamadı: %v | %s", err, m)
		}
	}

	pruneResolvedReports()
	fmt.Println("📦 Veritabanı tabloları ve şema güncellemeleri hazırlandı!")
}

func setupCORS(w *http.ResponseWriter, r *http.Request) bool {
	allowedOrigin := strings.TrimSpace(getEnv("ALLOWED_ORIGIN", "*"))
	reqOrigin := r.Header.Get("Origin")
	if allowedOrigin == "*" || allowedOrigin == "" {
		(*w).Header().Set("Access-Control-Allow-Origin", "*")
	} else if reqOrigin != "" && isOriginAllowed(reqOrigin) {
		(*w).Header().Set("Access-Control-Allow-Origin", reqOrigin)
		(*w).Header().Set("Vary", "Origin")
	} else {
		(*w).Header().Set("Access-Control-Allow-Origin", strings.Split(allowedOrigin, ",")[0])
	}
	(*w).Header().Set("Access-Control-Allow-Methods", "POST, GET, DELETE, OPTIONS")
	(*w).Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
	if r.Method == "OPTIONS" {
		(*w).WriteHeader(http.StatusOK)
		return true
	}
	return false
}

const termsRequiredMessage = "Devam etmek için Kullanım Koşulları ve Gizlilik Politikası'nı kabul etmelisiniz."

func handleRegister(w http.ResponseWriter, r *http.Request) {
	if setupCORS(&w, r) {
		return
	}
	if !applySecurityAndRateLimit(w, r, true) {
		return
	}

	var creds Credentials
	if err := json.NewDecoder(r.Body).Decode(&creds); err != nil {
		http.Error(w, "Geçersiz istek", http.StatusBadRequest)
		return
	}

	creds.Username = strings.TrimSpace(creds.Username)
	creds.Email = strings.ToLower(strings.TrimSpace(creds.Email))
	if len(creds.Username) < 2 || len(creds.Username) > 30 || len(creds.Password) < 6 {
		http.Error(w, "Kullanıcı adı (2-30 karakter) ve şifre (en az 6 karakter) olmalıdır", http.StatusBadRequest)
		return
	}
	if isForbiddenUsername(creds.Username) {
		http.Error(w, "Kullanıcı adı topluluk kurallarına aykırı ifadeler içeremez", http.StatusBadRequest)
		return
	}
	if len(creds.Password) > 72 {
		http.Error(w, "Şifre en fazla 72 karakter olabilir", http.StatusBadRequest)
		return
	}
	if creds.Email != "" && !validEmailAddress(creds.Email) {
		http.Error(w, "Geçerli bir e-posta adresi girin", http.StatusBadRequest)
		return
	}
	if !creds.AcceptedTerms {
		http.Error(w, termsRequiredMessage, http.StatusBadRequest)
		return
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(creds.Password), BcryptCost)
	if err != nil {
		http.Error(w, "Sunucu hatası", http.StatusInternalServerError)
		return
	}

	var userID int
	err = db.QueryRow("INSERT INTO users (username, password_hash, email, is_guest, terms_accepted_at) VALUES ($1, $2, $3, FALSE, NOW()) RETURNING id",
		creds.Username, string(hashedPassword), creds.Email).Scan(&userID)
	if err != nil {
		http.Error(w, "Bu kullanıcı adı zaten alınmış", http.StatusConflict)
		return
	}

	_, err = db.Exec("INSERT INTO profiles (user_id, xp, tokens, cases, unlocked_dice, selected_dice) VALUES ($1, 120, 210, 2, 'ivory', 'ivory')", userID)
	if err != nil {
		http.Error(w, "Profil oluşturulamadı", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(map[string]string{"message": "Kayıt başarılı!"})
}

func handleLogin(w http.ResponseWriter, r *http.Request) {
	if setupCORS(&w, r) {
		return
	}
	if !applySecurityAndRateLimit(w, r, true) {
		return
	}

	var creds Credentials
	if err := json.NewDecoder(r.Body).Decode(&creds); err != nil {
		http.Error(w, "Geçersiz istek", http.StatusBadRequest)
		return
	}

	creds.Username = strings.TrimSpace(creds.Username)
	if locked, secs := checkLoginLocked(creds.Username); locked {
		http.Error(w, fmt.Sprintf("Çok fazla hatalı giriş denemesi. Lütfen %d saniye sonra tekrar deneyin.", secs), http.StatusTooManyRequests)
		return
	}

	var userID int
	var realUsername, storedHash string
	err := db.QueryRow("SELECT id, username, password_hash FROM users WHERE LOWER(username) = LOWER($1)", creds.Username).Scan(&userID, &realUsername, &storedHash)
	if err != nil {
		_ = bcrypt.CompareHashAndPassword(dummyBcrypt, []byte(creds.Password))
		recordLoginFailure(creds.Username)
		http.Error(w, "Kullanıcı bulunamadı veya şifre yanlış", http.StatusUnauthorized)
		return
	}

	if err = bcrypt.CompareHashAndPassword([]byte(storedHash), []byte(creds.Password)); err != nil {
		recordLoginFailure(creds.Username)
		http.Error(w, "Kullanıcı bulunamadı veya şifre yanlış", http.StatusUnauthorized)
		return
	}
	resetLoginFailures(creds.Username)
	if isUserBanned(userID) {
		http.Error(w, bannedAccountMessage, http.StatusForbidden)
		return
	}

	tokenString, err := issueJWTForUser(userID, 7)
	if err != nil {
		http.Error(w, "Token oluşturulamadı", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"token": tokenString, "username": realUsername})
}

type ProfileData struct {
	Username         string   `json:"username"`
	Avatar           string   `json:"avatar"`
	Country          string   `json:"country"`
	XP               int      `json:"xp"`
	Tokens           int      `json:"tokens"`
	Cases            int      `json:"cases"`
	Wins             int      `json:"wins"`
	Games            int      `json:"gamesPlayed"`
	HighScore        int      `json:"highScore"`
	UnlockedDice     []string `json:"unlockedDice"`
	SelectedDice     string   `json:"selectedDice"`
	LoginStreak      int      `json:"loginStreak"`
	LastDailyReward  string   `json:"lastDailyReward"`
	UpperBonusesWon  int      `json:"upperBonusesWon"`
	TotalYachts      int      `json:"totalYachts"`
	MaxYachtsInMatch int      `json:"maxYachtsInMatch"`
	ClaimedQuests    []string `json:"claimedQuests"`
	ActiveMatch      bool     `json:"activeMatch"`
}

func parseTokenString(tokenString string) (string, error) {
	claims := &jwt.RegisteredClaims{}
	token, err := jwt.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("beklenmeyen imza algoritması: %v", token.Header["alg"])
		}
		return jwtKey, nil
	})
	if err != nil || !token.Valid {
		return "", fmt.Errorf("geçersiz token")
	}
	uid, err := strconv.Atoi(claims.Subject)
	if err != nil || claims.IssuedAt == nil {
		return "", fmt.Errorf("geçersiz token")
	}
	if err := ensureSessionActive(uid, claims.IssuedAt.Time); err != nil {
		return "", err
	}
	touchUserPresence(uid)
	return claims.Subject, nil
}

func getUserIDFromToken(r *http.Request) (string, error) {
	authHeader := r.Header.Get("Authorization")
	if authHeader == "" {
		return "", fmt.Errorf("yetkisiz")
	}
	tokenString := strings.TrimPrefix(authHeader, "Bearer ")
	return parseTokenString(tokenString)
}

func fetchProfileByUserID(userID string) (ProfileData, error) {
	var p ProfileData
	var unlockedStr, claimedStr string
	err := db.QueryRow(`
		SELECT u.username, COALESCE(p.avatar, ''), COALESCE(p.country, 'TR'),
		       p.xp, p.tokens, p.cases, p.wins, p.games_played, p.high_score,
		       COALESCE(p.unlocked_dice, 'ivory'), COALESCE(p.selected_dice, 'ivory'),
		       COALESCE(p.login_streak, 0), COALESCE(p.last_daily_reward, ''),
		       COALESCE(p.upper_bonuses_won, 0), COALESCE(p.total_yachts, 0),
		       COALESCE(p.max_yachts_in_match, 0), COALESCE(p.claimed_quests, '')
		FROM profiles p
		JOIN users u ON u.id = p.user_id
		WHERE p.user_id = $1
	`, userID).Scan(
		&p.Username, &p.Avatar, &p.Country,
		&p.XP, &p.Tokens, &p.Cases, &p.Wins, &p.Games, &p.HighScore,
		&unlockedStr, &p.SelectedDice, &p.LoginStreak, &p.LastDailyReward,
		&p.UpperBonusesWon, &p.TotalYachts, &p.MaxYachtsInMatch, &claimedStr,
	)
	if err != nil {
		return p, err
	}
	if unlockedStr == "" {
		p.UnlockedDice = []string{"ivory"}
	} else {
		p.UnlockedDice = strings.Split(unlockedStr, ",")
	}
	if claimedStr == "" {
		p.ClaimedQuests = []string{}
	} else {
		p.ClaimedQuests = strings.Split(claimedStr, ",")
	}
	if p.Country == "" {
		p.Country = "TR"
	}
	return p, nil
}

func handleGetProfile(w http.ResponseWriter, r *http.Request) {
	if setupCORS(&w, r) {
		return
	}
	if !applySecurityAndRateLimit(w, r, false) {
		return
	}

	userID, err := getUserIDFromToken(r)
	if err != nil {
		http.Error(w, "Yetkisiz", http.StatusUnauthorized)
		return
	}

	p, err := fetchProfileByUserID(userID)
	if err != nil {
		http.Error(w, "Profil bulunamadı", http.StatusNotFound)
		return
	}
	if uid, err := strconv.Atoi(userID); err == nil {
		p.ActiveMatch = activeGameFor(uid) != nil
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(p)
}

// POST /profile/update : Kullanıcı adını, profil resmini, ülkesini ve kuşanılan zarı güvenli şekilde günceller
func handleUpdateProfile(w http.ResponseWriter, r *http.Request) {
	if setupCORS(&w, r) {
		return
	}
	if !applySecurityAndRateLimit(w, r, false) {
		return
	}

	userID, err := getUserIDFromToken(r)
	if err != nil {
		http.Error(w, "Yetkisiz", http.StatusUnauthorized)
		return
	}

	var req struct {
		Username     string  `json:"username"`
		SelectedDice string  `json:"selectedDice"`
		Avatar       *string `json:"avatar"`
		Country      string  `json:"country"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Geçersiz veri", http.StatusBadRequest)
		return
	}

	if trimmed := strings.TrimSpace(req.Username); len(trimmed) >= 2 && len(trimmed) <= 30 {
		if isForbiddenUsername(trimmed) {
			http.Error(w, "Kullanıcı adı topluluk kurallarına aykırı ifadeler içeremez", http.StatusBadRequest)
			return
		}
		_, err = db.Exec("UPDATE users SET username = $1 WHERE id = $2", trimmed, userID)
		if err != nil {
			http.Error(w, "Bu kullanıcı adı zaten kullanılıyor", http.StatusConflict)
			return
		}
	}

	if req.Avatar != nil {
		av := strings.TrimSpace(*req.Avatar)
		if len(av) <= 180000 { // Maksimum ~180KB Base64 resim veya emoji/preset
			_, _ = db.Exec("UPDATE profiles SET avatar = $1 WHERE user_id = $2", av, userID)
		}
	}

	if country := strings.ToUpper(strings.TrimSpace(req.Country)); len(country) >= 2 && len(country) <= 6 {
		_, _ = db.Exec("UPDATE profiles SET country = $1 WHERE user_id = $2", country, userID)
	}

	if req.SelectedDice != "" {
		p, err := fetchProfileByUserID(userID)
		if err == nil {
			for _, u := range p.UnlockedDice {
				if u == req.SelectedDice {
					_, _ = db.Exec("UPDATE profiles SET selected_dice = $1 WHERE user_id = $2", req.SelectedDice, userID)
					break
				}
			}
		}
	}

	p, err := fetchProfileByUserID(userID)
	if err != nil {
		http.Error(w, "Profil okunamadı", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(p)
}

// POST /profile/claim-daily : Günlük ödülü sunucu saatiyle doğrular ve verir
func handleClaimDaily(w http.ResponseWriter, r *http.Request) {
	if setupCORS(&w, r) {
		return
	}
	if !applySecurityAndRateLimit(w, r, false) {
		return
	}

	userID, err := getUserIDFromToken(r)
	if err != nil {
		http.Error(w, "Yetkisiz", http.StatusUnauthorized)
		return
	}

	p, err := fetchProfileByUserID(userID)
	if err != nil {
		http.Error(w, "Profil bulunamadı", http.StatusNotFound)
		return
	}

	today := time.Now().UTC().Format("2006-01-02")
	if p.LastDailyReward == today {
		http.Error(w, "Bugünkü günlük ödül zaten alındı", http.StatusBadRequest)
		return
	}

	yesterday := time.Now().UTC().AddDate(0, 0, -1).Format("2006-01-02")
	newStreak := 1
	if p.LastDailyReward == yesterday {
		newStreak = p.LoginStreak + 1
	}

	cappedStreak := newStreak
	if cappedStreak > 30 {
		cappedStreak = 30
	}
	rewardXP := 100 + (cappedStreak * 20)
	rewardTokens := 50 + (cappedStreak * 10)

	res, err := db.Exec(`
		UPDATE profiles
		SET xp = xp + $1, tokens = tokens + $2, login_streak = $3, last_daily_reward = $4
		WHERE user_id = $5 AND COALESCE(last_daily_reward, '') <> $4
	`, rewardXP, rewardTokens, newStreak, today, userID)
	if err != nil {
		http.Error(w, "Ödül kaydedilemedi", http.StatusInternalServerError)
		return
	}
	if rows, _ := res.RowsAffected(); rows == 0 {
		http.Error(w, "Bugünkü günlük ödül zaten alındı", http.StatusBadRequest)
		return
	}

	updated, _ := fetchProfileByUserID(userID)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"profile":      updated,
		"rewardXp":     rewardXP,
		"rewardTokens": rewardTokens,
		"streak":       newStreak,
	})
}

// POST /profile/open-case : Kasa açma işlemini sunucu tarafında atomik olarak yapar
func handleOpenCase(w http.ResponseWriter, r *http.Request) {
	if setupCORS(&w, r) {
		return
	}
	if !applySecurityAndRateLimit(w, r, false) {
		return
	}

	userID, err := getUserIDFromToken(r)
	if err != nil {
		http.Error(w, "Yetkisiz", http.StatusUnauthorized)
		return
	}

	reward := secureRandInt(150) + 50 // 50..199 arası jeton

	res, err := db.Exec(`
		UPDATE profiles
		SET cases = cases - 1, tokens = tokens + $1
		WHERE user_id = $2 AND cases > 0
	`, reward, userID)
	if err != nil {
		http.Error(w, "Sunucu hatası", http.StatusInternalServerError)
		return
	}
	rows, _ := res.RowsAffected()
	if rows == 0 {
		http.Error(w, "Açılacak kasa yok", http.StatusBadRequest)
		return
	}

	updated, _ := fetchProfileByUserID(userID)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"reward":  reward,
		"profile": updated,
	})
}

// POST /shop/buy : Marketten zar satın alma işlemini sunucuda doğrular
func handleBuyDice(w http.ResponseWriter, r *http.Request) {
	if setupCORS(&w, r) {
		return
	}
	if !applySecurityAndRateLimit(w, r, false) {
		return
	}

	userID, err := getUserIDFromToken(r)
	if err != nil {
		http.Error(w, "Yetkisiz", http.StatusUnauthorized)
		return
	}

	var req struct {
		Dice string `json:"dice"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Geçersiz istek", http.StatusBadRequest)
		return
	}

	price, exists := dicePrices[req.Dice]
	if !exists {
		http.Error(w, "Geçersiz zar tipi", http.StatusBadRequest)
		return
	}

	p, err := fetchProfileByUserID(userID)
	if err != nil {
		http.Error(w, "Profil bulunamadı", http.StatusNotFound)
		return
	}

	for _, u := range p.UnlockedDice {
		if u == req.Dice {
			// Zaten sahipse sadece seçili zar olarak işaretle
			_, _ = db.Exec("UPDATE profiles SET selected_dice = $1 WHERE user_id = $2", req.Dice, userID)
			updated, _ := fetchProfileByUserID(userID)
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(updated)
			return
		}
	}

	if p.Tokens < price {
		http.Error(w, "Yetersiz jeton!", http.StatusBadRequest)
		return
	}

	newUnlocked := append(p.UnlockedDice, req.Dice)
	unlockedStr := strings.Join(newUnlocked, ",")

	res, err := db.Exec(`
		UPDATE profiles
		SET tokens = tokens - $1, unlocked_dice = $2, selected_dice = $3
		WHERE user_id = $4 AND tokens >= $1
	`, price, unlockedStr, req.Dice, userID)
	if err != nil {
		http.Error(w, "Satın alma başarısız", http.StatusInternalServerError)
		return
	}
	if rows, _ := res.RowsAffected(); rows == 0 {
		http.Error(w, "Yetersiz jeton!", http.StatusBadRequest)
		return
	}

	updated, _ := fetchProfileByUserID(userID)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(updated)
}

// ─── ARKADAŞLIK VE OYUN DAVETİ SİSTEMİ ───────────────────────────────────────

type FriendItem struct {
	FriendshipID int    `json:"friendshipId"`
	UserID       int    `json:"userId"`
	Username     string `json:"username"`
	Avatar       string `json:"avatar"`
	Country      string `json:"country"`
	XP           int    `json:"xp"`
	Wins         int    `json:"wins"`
	IsOnline     bool   `json:"isOnline"`
	InGame       bool   `json:"inGame"`
}

// GET /friends : Kullanıcının arkadaşlarını, bekleyen isteklerini ve aktif oyun davetlerini döner
func handleGetFriends(w http.ResponseWriter, r *http.Request) {
	if setupCORS(&w, r) {
		return
	}
	if !applySecurityAndRateLimit(w, r, false) {
		return
	}

	uidStr, err := getUserIDFromToken(r)
	if err != nil {
		http.Error(w, "Yetkisiz", http.StatusUnauthorized)
		return
	}
	myID, _ := strconv.Atoi(uidStr)
	touchUserPresence(myID)

	rows, err := db.Query(`
		SELECT f.id, f.requester_id, f.addressee_id, f.status,
		       u1.username, COALESCE(p1.avatar, ''), COALESCE(p1.country, 'TR'), p1.xp, p1.wins,
		       u2.username, COALESCE(p2.avatar, ''), COALESCE(p2.country, 'TR'), p2.xp, p2.wins
		FROM friendships f
		JOIN users u1 ON u1.id = f.requester_id
		JOIN profiles p1 ON p1.user_id = f.requester_id
		JOIN users u2 ON u2.id = f.addressee_id
		JOIN profiles p2 ON p2.user_id = f.addressee_id
		WHERE f.requester_id = $1 OR f.addressee_id = $1
		ORDER BY f.id DESC
	`, myID)
	if err != nil {
		http.Error(w, "Arkadaş listesi alınamadı", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	friends := []FriendItem{}
	incoming := []FriendItem{}
	outgoing := []FriendItem{}

	for rows.Next() {
		var fid, reqID, addID int
		var status string
		var u1Name, p1Av, p1Country string
		var p1XP, p1Wins int
		var u2Name, p2Av, p2Country string
		var p2XP, p2Wins int

		if err := rows.Scan(&fid, &reqID, &addID, &status, &u1Name, &p1Av, &p1Country, &p1XP, &p1Wins, &u2Name, &p2Av, &p2Country, &p2XP, &p2Wins); err != nil {
			continue
		}

		if status == "accepted" {
			if reqID == myID {
				on, ig := getUserOnlineState(addID)
				friends = append(friends, FriendItem{FriendshipID: fid, UserID: addID, Username: u2Name, Avatar: p2Av, Country: p2Country, XP: p2XP, Wins: p2Wins, IsOnline: on, InGame: ig})
			} else {
				on, ig := getUserOnlineState(reqID)
				friends = append(friends, FriendItem{FriendshipID: fid, UserID: reqID, Username: u1Name, Avatar: p1Av, Country: p1Country, XP: p1XP, Wins: p1Wins, IsOnline: on, InGame: ig})
			}
		} else if status == "pending" {
			if addID == myID {
				on, ig := getUserOnlineState(reqID)
				incoming = append(incoming, FriendItem{FriendshipID: fid, UserID: reqID, Username: u1Name, Avatar: p1Av, Country: p1Country, XP: p1XP, Wins: p1Wins, IsOnline: on, InGame: ig})
			} else {
				on, ig := getUserOnlineState(addID)
				outgoing = append(outgoing, FriendItem{FriendshipID: fid, UserID: addID, Username: u2Name, Avatar: p2Av, Country: p2Country, XP: p2XP, Wins: p2Wins, IsOnline: on, InGame: ig})
			}
		}
	}

	// Aktif ve hala açık olan özel odalara ait oyun davetlerini filtrele
	mu.Lock()
	rawInvites := pendingInvites[myID]
	validInvites := []GameInvite{}
	now := time.Now()
	for _, inv := range rawInvites {
		if entry, exists := privateRooms[inv.RoomCode]; exists && entry.Host != nil && now.Sub(inv.CreatedAt) < 5*time.Minute {
			validInvites = append(validInvites, inv)
		}
	}
	pendingInvites[myID] = validInvites
	mu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"friends":  friends,
		"incoming": incoming,
		"outgoing": outgoing,
		"invites":  validInvites,
	})
}

// POST /friends/request : Kullanıcı adına göre arkadaşlık isteği gönderir
func handleFriendRequest(w http.ResponseWriter, r *http.Request) {
	if setupCORS(&w, r) {
		return
	}
	if !applySecurityAndRateLimit(w, r, false) {
		return
	}

	uidStr, err := getUserIDFromToken(r)
	if err != nil {
		http.Error(w, "Yetkisiz", http.StatusUnauthorized)
		return
	}
	myID, _ := strconv.Atoi(uidStr)

	var req struct {
		Username string `json:"username"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Geçersiz istek", http.StatusBadRequest)
		return
	}

	targetName := strings.TrimSpace(req.Username)
	if targetName == "" {
		http.Error(w, "Kullanıcı adı boş olamaz", http.StatusBadRequest)
		return
	}

	var targetID int
	err = db.QueryRow("SELECT id FROM users WHERE LOWER(username) = LOWER($1)", targetName).Scan(&targetID)
	if err != nil {
		http.Error(w, "Bu isimde bir oyuncu bulunamadı!", http.StatusNotFound)
		return
	}

	if targetID == myID {
		http.Error(w, "Kendinizi arkadaş olarak ekleyemezsiniz!", http.StatusBadRequest)
		return
	}
	if isBlockedEither(myID, targetID) {
		http.Error(w, "Bu isimde bir oyuncu bulunamadı!", http.StatusNotFound)
		return
	}

	// Önceden ekli mi veya bekleyen istek var mı kontrol et
	var existingID int
	var existingStatus string
	err = db.QueryRow(`
		SELECT id, status FROM friendships
		WHERE (requester_id = $1 AND addressee_id = $2) OR (requester_id = $2 AND addressee_id = $1)
	`, myID, targetID).Scan(&existingID, &existingStatus)

	if err == nil {
		if existingStatus == "accepted" {
			http.Error(w, "Bu oyuncu zaten arkadaş listenizde!", http.StatusConflict)
			return
		}
		http.Error(w, "Zaten bekleyen bir arkadaşlık isteği var!", http.StatusConflict)
		return
	}

	_, err = db.Exec("INSERT INTO friendships (requester_id, addressee_id, status) VALUES ($1, $2, 'pending')", myID, targetID)
	if err != nil {
		http.Error(w, "Arkadaşlık isteği gönderilemedi", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"message": "Arkadaşlık isteği gönderildi!"})
}

// POST /friends/respond : Gelen arkadaşlık isteğini kabul eder, reddeder veya arkadaşı siler
func handleFriendRespond(w http.ResponseWriter, r *http.Request) {
	if setupCORS(&w, r) {
		return
	}
	if !applySecurityAndRateLimit(w, r, false) {
		return
	}

	uidStr, err := getUserIDFromToken(r)
	if err != nil {
		http.Error(w, "Yetkisiz", http.StatusUnauthorized)
		return
	}
	myID, _ := strconv.Atoi(uidStr)

	var req struct {
		FriendshipID int    `json:"friendshipId"`
		Action       string `json:"action"` // "accept", "reject", "remove"
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Geçersiz istek", http.StatusBadRequest)
		return
	}

	if req.Action == "accept" {
		_, err = db.Exec("UPDATE friendships SET status = 'accepted' WHERE id = $1 AND addressee_id = $2", req.FriendshipID, myID)
	} else {
		_, err = db.Exec("DELETE FROM friendships WHERE id = $1 AND (addressee_id = $2 OR requester_id = $2)", req.FriendshipID, myID)
	}

	if err != nil {
		http.Error(w, "İşlem başarısız", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// POST /friends/invite : Arkadaşa özel oda daveti gönderir veya daveti temizler
func handleFriendInvite(w http.ResponseWriter, r *http.Request) {
	if setupCORS(&w, r) {
		return
	}
	if !applySecurityAndRateLimit(w, r, false) {
		return
	}

	uidStr, err := getUserIDFromToken(r)
	if err != nil {
		http.Error(w, "Yetkisiz", http.StatusUnauthorized)
		return
	}
	myID, _ := strconv.Atoi(uidStr)

	var req struct {
		FriendUserID int    `json:"friendUserId"`
		RoomCode     string `json:"roomCode"`
		DismissID    int    `json:"dismissId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Geçersiz istek", http.StatusBadRequest)
		return
	}

	// Daveti reddetme / kapatma
	if req.DismissID > 0 {
		mu.Lock()
		list := pendingInvites[myID]
		filtered := []GameInvite{}
		for _, inv := range list {
			if inv.ID != req.DismissID {
				filtered = append(filtered, inv)
			}
		}
		pendingInvites[myID] = filtered
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "dismissed"})
		return
	}

	if !areFriends(myID, req.FriendUserID) || isBlockedEither(myID, req.FriendUserID) {
		http.Error(w, "Yalnızca arkadaşlarınızı davet edebilirsiniz", http.StatusForbidden)
		return
	}

	var myName string
	_ = db.QueryRow("SELECT username FROM users WHERE id = $1", myID).Scan(&myName)
	if myName == "" {
		myName = "Arkadaşın"
	}

	mu.Lock()
	// Aynı odaya ait eski daveti varsa yenile
	list := pendingInvites[req.FriendUserID]
	filtered := []GameInvite{}
	for _, inv := range list {
		if inv.FromUserID != myID {
			filtered = append(filtered, inv)
		}
	}
	inviteSeq++
	filtered = append(filtered, GameInvite{
		ID:         inviteSeq,
		RoomCode:   strings.ToUpper(strings.TrimSpace(req.RoomCode)),
		FromUserID: myID,
		FromName:   myName,
		CreatedAt:  time.Now(),
	})
	pendingInvites[req.FriendUserID] = filtered
	mu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"message": "Oyun daveti gönderildi!"})
}

func main() {
	initDB()
	startRoomCleanupWorker()

	http.HandleFunc("/register", handleRegister)
	http.HandleFunc("/login", handleLogin)
	http.HandleFunc("/auth/guest", handleGuestAuth)
	http.HandleFunc("/auth/google", handleGoogleAuth)
	http.HandleFunc("/auth/apple", handleAppleAuth)
	http.HandleFunc("/auth/forgot-password", handleForgotPassword)
	http.HandleFunc("/auth/reset-password", handleResetPassword)
	http.HandleFunc("/profile", handleGetProfile)
	http.HandleFunc("/profile/update", handleUpdateProfile)
	http.HandleFunc("/profile/delete", handleDeleteAccount)
	http.HandleFunc("/profile/claim-daily", handleClaimDaily)
	http.HandleFunc("/profile/open-case", handleOpenCase)
	http.HandleFunc("/quests/claim", handleClaimQuest)
	http.HandleFunc("/shop/buy", handleBuyDice)
	http.HandleFunc("/friends", handleGetFriends)
	http.HandleFunc("/friends/request", handleFriendRequest)
	http.HandleFunc("/friends/respond", handleFriendRespond)
	http.HandleFunc("/friends/invite", handleFriendInvite)
	http.HandleFunc("/ugc/report", handleReportUser)
	http.HandleFunc("/ugc/block", handleBlockUser)
	http.HandleFunc("/ugc/unblock", handleUnblockUser)
	http.HandleFunc("/ugc/blocks", handleListBlocks)
	http.HandleFunc("/account/delete", handleWebAccountDeletion)
	http.HandleFunc("/admin", handleAdminPage)
	http.HandleFunc("/admin/queue", handleAdminQueue)
	http.HandleFunc("/admin/action", handleAdminAction)
	http.HandleFunc("/privacy", handlePrivacyHTML)
	http.HandleFunc("/terms", handleTermsHTML)
	http.HandleFunc("/leaderboard", handleLeaderboard)
	http.HandleFunc("/health", handleHealth)
	http.HandleFunc("/play", handleConnections)

	if !isDevelopment() && strings.TrimSpace(os.Getenv("ALLOWED_ORIGIN")) == "" {
		log.Println("⚠️ ALLOWED_ORIGIN tanımlı değil: web sitenizden gelen WebSocket bağlantıları reddedilecek.")
	}

	if !isDevelopment() {
		for key, warning := range map[string]string{
			"SMTP_HOST":           "şifre sıfırlama e-postaları gönderilemez",
			"ADMIN_TOKEN":         "moderasyon paneli kapalı kalır",
			"LEGAL_CONTACT_EMAIL": "hukuk metinlerinde iletişim adresi görünmez",
		} {
			if strings.TrimSpace(os.Getenv(key)) == "" {
				log.Printf("⚠️ %s tanımlı değil: %s.", key, warning)
			}
		}
	}

	port := getEnv("PORT", "8080")
	srv := &http.Server{
		Addr:              ":" + port,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	fmt.Printf("🚀 Yat Dice Sunucusu %s portunda aktif...\n", port)
	log.Fatal(srv.ListenAndServe())
}

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

// X-Forwarded-For istemci tarafından sahte doldurulabilir. Yalnızca önümüzdeki güvenilir
// proxy sayısı kadar sağdan sayılır; proxy yoksa (0) başlık tamamen yok sayılır.
func clientIPFrom(remoteAddr, forwardedFor string, trustedProxies int) string {
	if trustedProxies > 0 && forwardedFor != "" {
		parts := strings.Split(forwardedFor, ",")
		if len(parts) >= trustedProxies {
			if ip := strings.TrimSpace(parts[len(parts)-trustedProxies]); ip != "" {
				return ip
			}
		}
	}
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		return remoteAddr
	}
	return host
}

func getClientIP(r *http.Request) string {
	trustedProxies, _ := strconv.Atoi(os.Getenv("TRUSTED_PROXY_COUNT"))
	return clientIPFrom(r.RemoteAddr, r.Header.Get("X-Forwarded-For"), trustedProxies)
}

// applySecurityAndRateLimit: Tüm HTTP uç noktalarına güvenlik başlıkları,
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

func handleGuestAuth(w http.ResponseWriter, r *http.Request) {
	if setupCORS(&w, r) {
		return
	}
	if !applySecurityAndRateLimit(w, r, true) {
		return
	}

	var req struct {
		DeviceID      string `json:"deviceId"`
		AcceptedTerms bool   `json:"acceptedTerms"`
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
	if err == nil && isUserBanned(userID) {
		http.Error(w, bannedAccountMessage, http.StatusForbidden)
		return
	}
	if err != nil {
		if !req.AcceptedTerms {
			http.Error(w, termsRequiredMessage, http.StatusBadRequest)
			return
		}
		for attempt := 0; attempt < 10; attempt++ {
			candidate := fmt.Sprintf("Misafir_%d", secureRandInt(90000)+10000)
			randomPass, _ := bcrypt.GenerateFromPassword([]byte(fmt.Sprintf("guest-%s-%d", deviceID, time.Now().UnixNano())), BcryptCost)
			err = db.QueryRow(`
				INSERT INTO users (username, password_hash, guest_device_id, is_guest, terms_accepted_at)
				VALUES ($1, $2, $3, TRUE, NOW())
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

	tokenString, err := issueJWTForUser(userID, 365)
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

var googleTokenInfoURL = "https://oauth2.googleapis.com/tokeninfo"

type googleIdentity struct {
	Sub   string
	Email string
	Name  string
}

// Google ID token'ını doğrular. allowedAudiences web, Android ve iOS istemci kimliklerini içerebilir.
func verifyGoogleIDToken(credential string, allowedAudiences []string) (googleIdentity, error) {
	httpClient := &http.Client{Timeout: 5 * time.Second}
	resp, err := httpClient.Get(googleTokenInfoURL + "?id_token=" + url.QueryEscape(credential))
	if err != nil {
		return googleIdentity{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return googleIdentity{}, fmt.Errorf("tokeninfo durumu: %d", resp.StatusCode)
	}

	var info struct {
		Iss           string      `json:"iss"`
		Aud           string      `json:"aud"`
		Sub           string      `json:"sub"`
		Email         string      `json:"email"`
		EmailVerified interface{} `json:"email_verified"`
		Name          string      `json:"name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return googleIdentity{}, err
	}

	audienceOK := false
	for _, aud := range allowedAudiences {
		if aud == info.Aud {
			audienceOK = true
			break
		}
	}
	if !audienceOK {
		return googleIdentity{}, fmt.Errorf("beklenmeyen istemci kimliği")
	}
	if info.Iss != "accounts.google.com" && info.Iss != "https://accounts.google.com" {
		return googleIdentity{}, fmt.Errorf("beklenmeyen token yayıncısı")
	}
	email := strings.ToLower(strings.TrimSpace(info.Email))
	if info.Sub == "" || email == "" || fmt.Sprint(info.EmailVerified) != "true" {
		return googleIdentity{}, fmt.Errorf("doğrulanmış Google kimliği yok")
	}
	return googleIdentity{Sub: info.Sub, Email: email, Name: strings.TrimSpace(info.Name)}, nil
}

func splitCSV(raw string) []string {
	items := []string{}
	for _, item := range strings.Split(raw, ",") {
		if trimmed := strings.TrimSpace(item); trimmed != "" {
			items = append(items, trimmed)
		}
	}
	return items
}

func handleGoogleAuth(w http.ResponseWriter, r *http.Request) {
	if setupCORS(&w, r) {
		return
	}
	if !applySecurityAndRateLimit(w, r, true) {
		return
	}

	audiences := splitCSV(os.Getenv("GOOGLE_CLIENT_ID"))
	if len(audiences) == 0 {
		http.Error(w, "Google ile giriş bu sunucuda etkin değil", http.StatusServiceUnavailable)
		return
	}

	var req struct {
		Credential    string `json:"credential"`
		AcceptedTerms bool   `json:"acceptedTerms"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Credential == "" {
		http.Error(w, "Google kimlik bilgisi gerekli", http.StatusBadRequest)
		return
	}

	identity, err := verifyGoogleIDToken(req.Credential, audiences)
	if err != nil {
		log.Println("Google token doğrulanamadı:", err)
		http.Error(w, "Google oturumu doğrulanamadı", http.StatusUnauthorized)
		return
	}
	email := identity.Email
	displayName := identity.Name
	googleID := identity.Sub

	var userID int
	var username string
	err = db.QueryRow(`SELECT id, username FROM users WHERE google_id = $1 LIMIT 1`, googleID).Scan(&userID, &username)
	if err == nil && isUserBanned(userID) {
		http.Error(w, bannedAccountMessage, http.StatusForbidden)
		return
	}

	// /register e-postaları doğrulanmadığı için hesaplar e-postaya göre otomatik bağlanmaz
	if err != nil {
		if !req.AcceptedTerms {
			http.Error(w, termsRequiredMessage, http.StatusBadRequest)
			return
		}
		var emailTaken bool
		if qErr := db.QueryRow(`SELECT EXISTS(SELECT 1 FROM users WHERE LOWER(email) = LOWER($1))`, email).Scan(&emailTaken); qErr != nil {
			http.Error(w, "Sunucu hatası", http.StatusInternalServerError)
			return
		}
		if emailTaken {
			http.Error(w, "Bu e-posta ile kayıtlı bir hesap var. Lütfen kullanıcı adı ve şifrenizle giriş yapın.", http.StatusConflict)
			return
		}
	}

	if err != nil {
		userID, username, err = createProviderUser("google_id", googleID, email, displayName)
		if err != nil {
			http.Error(w, "Google hesabı oluşturulamadı", http.StatusInternalServerError)
			return
		}
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
	resetCodes = make(map[int]*resetCodeItem)
)

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

	resp := map[string]interface{}{
		"message": "Bilgiler kayıtlı hesapla eşleşiyorsa doğrulama kodu e-postanıza gönderildi.",
	}

	var userID int
	var dbEmail string
	var isGuest bool
	err := db.QueryRow(`SELECT id, COALESCE(email, ''), COALESCE(is_guest, FALSE) FROM users WHERE LOWER(username) = LOWER($1)`, username).
		Scan(&userID, &dbEmail, &isGuest)

	if err == nil && !isGuest && dbEmail != "" && strings.EqualFold(dbEmail, email) {
		code := fmt.Sprintf("%06d", secureRandInt(900000)+100000)
		resetMu.Lock()
		resetCodes[userID] = &resetCodeItem{
			code:      code,
			expiresAt: time.Now().Add(10 * time.Minute),
			attempts:  0,
		}
		resetMu.Unlock()

		if isDevelopment() {
			resp["resetCode"] = code
		}
		go func() {
			body := "Yat Dice şifre sıfırlama kodunuz: " + code + "\n\nKod 10 dakika geçerlidir. Bu isteği siz yapmadıysanız e-postayı yok sayın."
			if err := sendEmail(dbEmail, "Yat Dice şifre sıfırlama kodu", body); err != nil {
				log.Println("Şifre sıfırlama e-postası gönderilemedi:", err)
			}
		}()
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

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

	// Şifre değişince eski oturumlar geçersiz olur
	if _, err := db.Exec(`UPDATE users SET password_hash = $1, tokens_valid_after = date_trunc('second', NOW()) WHERE id = $2`, string(hashed), userID); err != nil {
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
	userInGame   = make(map[int]int)
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

// ─── 6. GÖREV MODU (NORMAL & ZOR GÖREVLER — XP, ALTIN VE KASA ÖDÜLLÜ) ────────

type questRewardDef struct {
	xp     int
	tokens int
	cases  int
	check  func(p ProfileData) bool
}

var questDefinitions = map[string]questRewardDef{
	"first_match": {
		xp: 150, tokens: 75, cases: 0,
		check: func(p ProfileData) bool { return p.Games >= 1 },
	},
	"upper_bonus_1": {
		xp: 300, tokens: 150, cases: 0,
		check: func(p ProfileData) bool { return p.UpperBonusesWon >= 1 },
	},
	"first_yacht": {
		xp: 350, tokens: 200, cases: 0,
		check: func(p ProfileData) bool { return p.TotalYachts >= 1 },
	},
	"wins_3": {
		xp: 400, tokens: 250, cases: 0,
		check: func(p ProfileData) bool { return p.Wins >= 3 },
	},
	"score_220": {
		xp: 600, tokens: 400, cases: 1,
		check: func(p ProfileData) bool { return p.HighScore >= 220 },
	},
	"upper_bonus_3": {
		xp: 750, tokens: 500, cases: 1,
		check: func(p ProfileData) bool { return p.UpperBonusesWon >= 3 },
	},
	"double_yacht": {
		xp: 1000, tokens: 750, cases: 2,
		check: func(p ProfileData) bool { return p.MaxYachtsInMatch >= 2 },
	},
	"triple_yacht": {
		xp: 2000, tokens: 1500, cases: 3,
		check: func(p ProfileData) bool { return p.MaxYachtsInMatch >= 3 },
	},
}

func handleClaimQuest(w http.ResponseWriter, r *http.Request) {
	if setupCORS(&w, r) {
		return
	}
	if !applySecurityAndRateLimit(w, r, false) {
		return
	}

	userID, err := getUserIDFromToken(r)
	if err != nil {
		http.Error(w, "Yetkisiz", http.StatusUnauthorized)
		return
	}

	var req struct {
		QuestID string `json:"questId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Geçersiz istek", http.StatusBadRequest)
		return
	}

	qDef, exists := questDefinitions[req.QuestID]
	if !exists {
		http.Error(w, "Geçersiz görev", http.StatusBadRequest)
		return
	}

	p, err := fetchProfileByUserID(userID)
	if err != nil {
		http.Error(w, "Profil bulunamadı", http.StatusNotFound)
		return
	}

	for _, c := range p.ClaimedQuests {
		if c == req.QuestID {
			http.Error(w, "Bu görev ödülü zaten alındı", http.StatusConflict)
			return
		}
	}

	if !qDef.check(p) {
		http.Error(w, "Görev şartları henüz tamamlanmadı", http.StatusBadRequest)
		return
	}

	newClaimed := append(p.ClaimedQuests, req.QuestID)
	claimedCSV := strings.Join(newClaimed, ",")

	_, err = db.Exec(`
		UPDATE profiles
		SET xp = xp + $1,
		    tokens = tokens + $2,
		    cases = cases + $3,
		    claimed_quests = $4
		WHERE user_id = $5
	`, qDef.xp, qDef.tokens, qDef.cases, claimedCSV, userID)
	if err != nil {
		http.Error(w, "Görev ödülü kaydedilemedi", http.StatusInternalServerError)
		return
	}

	updated, _ := fetchProfileByUserID(userID)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"questId":      req.QuestID,
		"rewardXp":     qDef.xp,
		"rewardTokens": qDef.tokens,
		"rewardCases":  qDef.cases,
		"profile":      updated,
	})
}

// ─── 7. LİDERLİK TABLOSU (LEADERBOARD) VE SAĞLIK KONTROLÜ (HEALTH CHECK) ──────

type LeaderboardEntry struct {
	Rank      int    `json:"rank"`
	Username  string `json:"username"`
	Avatar    string `json:"avatar"`
	Country   string `json:"country"`
	XP        int    `json:"xp"`
	Wins      int    `json:"wins"`
	HighScore int    `json:"highScore"`
}

// GET /leaderboard : En yüksek galibiyet ve XP'ye sahip ilk 20 oyuncuyu döndürür
func handleLeaderboard(w http.ResponseWriter, r *http.Request) {
	if setupCORS(&w, r) {
		return
	}
	if !applySecurityAndRateLimit(w, r, false) {
		return
	}

	rows, err := db.Query(`
		SELECT u.username, COALESCE(p.avatar, ''), COALESCE(p.country, 'TR'),
		       p.xp, p.wins, p.high_score
		FROM profiles p
		JOIN users u ON u.id = p.user_id
		WHERE COALESCE(u.is_banned, FALSE) = FALSE
		ORDER BY p.wins DESC, p.xp DESC, p.high_score DESC
		LIMIT 20
	`)
	if err != nil {
		http.Error(w, "Liderlik tablosu alınamadı", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	leaders := []LeaderboardEntry{}
	rank := 1
	for rows.Next() {
		var item LeaderboardEntry
		if err := rows.Scan(&item.Username, &item.Avatar, &item.Country, &item.XP, &item.Wins, &item.HighScore); err == nil {
			item.Rank = rank
			rank++
			leaders = append(leaders, item)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"leaders": leaders,
	})
}

// GET /health : Sunucu ve veritabanı sağlık durumu kontrolü (Docker/Render/Railway için)
func handleHealth(w http.ResponseWriter, r *http.Request) {
	if setupCORS(&w, r) {
		return
	}
	status, dbStatus, code := "ok", "ok", http.StatusOK
	if err := db.Ping(); err != nil {
		status, dbStatus, code = "error", "error", http.StatusServiceUnavailable
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"status": status,
		"db":     dbStatus,
		"time":   time.Now().Format(time.RFC3339),
	})
}
