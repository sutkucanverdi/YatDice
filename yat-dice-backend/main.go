package main

import (
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"math/big"
	"net/http"
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
	BcryptCost          = 10 // Cost 14 DoS riskine yol açtığı için 10'a çekildi
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
}

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		origin := r.Header.Get("Origin")
		if origin == "" {
			return true
		}
		allowedOrigin := os.Getenv("ALLOWED_ORIGIN")
		if allowedOrigin != "" {
			return origin == allowedOrigin
		}
		return strings.HasPrefix(origin, "http://localhost:") || strings.HasPrefix(origin, "http://127.0.0.1:")
	},
}

type Client struct {
	ID       string
	UserID   int // Veritabanındaki kullanıcı ID'si (Misafir ise 0)
	Conn     *websocket.Conn
	WriteMu  sync.Mutex // Concurrent websocket write panic önleyici
	Room     *Room
	Username string
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
	CreatedAt time.Time
}

type Room struct {
	ID            string
	Player1       *Client
	Player2       *Client
	IsBotMatch    bool
	Mu            sync.Mutex // Oda içi oyun durumu kilidi (Global mutex darboğazını önler)
	Turn          string     // "p1" veya "p2"
	Round         int        // 1..13
	RollsLeft     int        // 3..0
	Dice          [5]int
	Locked        [5]bool
	P1Scores      map[string]int
	P2Scores      map[string]int
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
	waitingClient  *Client
	privateRooms   = make(map[string]PrivateRoomEntry)
	pendingInvites = make(map[int][]GameInvite) // targetUserID -> gelen oyun davetleri
	inviteSeq      int
	mu             sync.Mutex // Sadece eşleştirme (matchmaking) kuyruğunu ve davetleri korur
	roomCounter    int
	db             *sql.DB
	jwtKey         = []byte(getEnv("JWT_SECRET", "dev-secret-change-in-production"))
)

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

type Credentials struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Email    string `json:"email"`
}

func initDB() {
	connStr := getEnv("DATABASE_URL", "host=localhost port=5432 user=yatzy_user password=yatzy_password dbname=yatzy_gamedb sslmode=disable")

	var err error
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

// Veritabanında maç sonu ödüllerini güvenli şekilde günceller
func awardMatchResultDB(userID int, won bool, score int, xpGain int, tokenGain int, caseGain int) {
	if userID <= 0 {
		return
	}
	winInc := 0
	if won {
		winInc = 1
	}
	_, err := db.Exec(`
		UPDATE profiles
		SET xp = xp + $1,
		    tokens = tokens + $2,
		    cases = cases + $3,
		    wins = wins + $4,
		    games_played = games_played + 1,
		    high_score = GREATEST(high_score, $5)
		WHERE user_id = $6
	`, xpGain, tokenGain, caseGain, winInc, score, userID)
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
	r.resetTurnTimerLocked()
	return r
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
		p1Won := p1Total >= p2Total
		p2Won := p2Total > p1Total

		p1XP, p1Tokens, p1Cases := 150, 40, 0
		if p1Won {
			p1XP, p1Tokens, p1Cases = 300, 100, 1
		}
		p2XP, p2Tokens, p2Cases := 150, 40, 0
		if p2Won {
			p2XP, p2Tokens, p2Cases = 300, 100, 1
		}

		if r.Player1 != nil {
			awardMatchResultDB(r.Player1.UserID, p1Won, p1Total, p1XP, p1Tokens, p1Cases)
		}
		if r.Player2 != nil {
			awardMatchResultDB(r.Player2.UserID, p2Won, p2Total, p2XP, p2Tokens, p2Cases)
		}

		msg["rewards"] = map[string]interface{}{
			"p1": map[string]int{"xp": p1XP, "tokens": p1Tokens, "cases": p1Cases},
			"p2": map[string]int{"xp": p2XP, "tokens": p2Tokens, "cases": p2Cases},
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

func handleConnections(w http.ResponseWriter, r *http.Request) {
	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println("WebSocket hatası:", err)
		return
	}

	// Token varsa kullanıcıyı doğrula
	tokenParam := r.URL.Query().Get("token")
	userID := 0
	if tokenParam != "" && !strings.HasPrefix(tokenParam, "guest_token_") {
		if uidStr, err := parseTokenString(tokenParam); err == nil {
			userID, _ = strconv.Atoi(uidStr)
		}
	}

	client := &Client{Conn: ws, UserID: userID}

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

		mu.Lock()
		if waitingClient == client {
			waitingClient = nil
		}
		for code, entry := range privateRooms {
			if entry.Host == client {
				delete(privateRooms, code)
			}
		}
		room := client.Room
		client.Room = nil
		mu.Unlock()
		if room != nil {
			setUserGameState(client.UserID, false)
		}

		// Oyuncu aktif ve bitmemiş bir oyundayken çıkarsa rakibine Hükmen Galibiyet ver
		if room != nil {
			room.Mu.Lock()
			if !room.GameOver {
				room.GameOver = true
				room.stopTurnTimerLocked()

				var opponent *Client
				if room.Player1 == client {
					opponent = room.Player2
				} else {
					opponent = room.Player1
				}

				if opponent != nil {
					awardMatchResultDB(opponent.UserID, true, 0, 150, 50, 0)
					_ = opponent.WriteJSON(map[string]interface{}{
						"type": "OPPONENT_LEFT",
						"reward": map[string]int{
							"xp":     150,
							"tokens": 50,
						},
					})
					opponent.Room = nil
				}
			}
			room.Mu.Unlock()
		}
	}()

	for {
		var msg map[string]interface{}
		err := ws.ReadJSON(&msg)
		if err != nil {
			break
		}
		_ = ws.SetReadDeadline(time.Now().Add(PongWait))

		if username, ok := msg["username"].(string); ok && strings.TrimSpace(username) != "" {
			client.Username = strings.TrimSpace(username)
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

			if action == "BOT_MATCH" {
				// Bota karşı anında tek oyunculu maç başlat
				if waitingClient == client {
					waitingClient = nil
				}
				roomCounter++
				roomID := fmt.Sprintf("BOT-ROOM-%d", roomCounter)
				room := newRoom(roomID, client, nil)
				room.IsBotMatch = true
				client.Room = room

				_ = client.WriteJSON(map[string]interface{}{
					"type":         "MATCH_FOUND",
					"room":         roomID,
					"playerIndex":  1,
					"opponentName": "🤖 YatBot",
					"turnSeconds":  TurnDurationSeconds,
				})

			} else if action == "QUICK_MATCH" {
				// Aynı oyuncunun kendi kendisiyle eşleşmesini önle
				if waitingClient == nil || waitingClient == client {
					waitingClient = client
					_ = client.WriteJSON(map[string]interface{}{"type": "WAITING_FOR_OPPONENT"})
				} else {
					opponent := waitingClient
					waitingClient = nil

					roomCounter++
					roomID := fmt.Sprintf("ROOM-%d", roomCounter)
					room := newRoom(roomID, opponent, client)
					opponent.Room = room
					client.Room = room

					_ = opponent.WriteJSON(map[string]interface{}{
						"type":         "MATCH_FOUND",
						"room":         roomID,
						"playerIndex":  1,
						"opponentName": client.Username,
						"turnSeconds":  TurnDurationSeconds,
					})
					_ = client.WriteJSON(map[string]interface{}{
						"type":         "MATCH_FOUND",
						"room":         roomID,
						"playerIndex":  2,
						"opponentName": opponent.Username,
						"turnSeconds":  TurnDurationSeconds,
					})
				}

			} else if action == "CREATE_ROOM" {
				// Önceden açtığı özel oda varsa temizle (Duplicate room leak önleyici)
				for code, entry := range privateRooms {
					if entry.Host == client {
						delete(privateRooms, code)
					}
				}
				code := generateUniqueRoomCode()
				privateRooms[code] = PrivateRoomEntry{Host: client, CreatedAt: time.Now()}
				_ = client.WriteJSON(map[string]interface{}{"type": "ROOM_CREATED", "roomCode": code})

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

				if exists && entry.Host != nil && entry.Host != client {
					delete(privateRooms, code)
					host := entry.Host

					roomCounter++
					roomID := fmt.Sprintf("ROOM-%d", roomCounter)
					room := newRoom(roomID, host, client)
					host.Room = room
					client.Room = room

					_ = host.WriteJSON(map[string]interface{}{
						"type":         "MATCH_FOUND",
						"room":         roomID,
						"playerIndex":  1,
						"opponentName": client.Username,
						"turnSeconds":  TurnDurationSeconds,
					})
					_ = client.WriteJSON(map[string]interface{}{
						"type":         "MATCH_FOUND",
						"room":         roomID,
						"playerIndex":  2,
						"opponentName": host.Username,
						"turnSeconds":  TurnDurationSeconds,
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

		switch msgType {
		case "EMOJI":
			// Hızlı Emoji / Reaksiyon gönderimi (Sıra beklemeden gönderilebilir)
			emoji, ok := msg["emoji"].(string)
			if ok && len(emoji) <= 16 {
				room.broadcastLocked(map[string]interface{}{
					"type":   "EMOJI",
					"emoji":  emoji,
					"sender": playerRole,
				})
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
	}
	for _, m := range migrations {
		_, _ = db.Exec(m)
	}

	fmt.Println("📦 Veritabanı tabloları ve şema güncellemeleri hazırlandı!")
}

func setupCORS(w *http.ResponseWriter, r *http.Request) bool {
	allowedOrigin := getEnv("ALLOWED_ORIGIN", "*")
	(*w).Header().Set("Access-Control-Allow-Origin", allowedOrigin)
	(*w).Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS")
	(*w).Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
	if r.Method == "OPTIONS" {
		(*w).WriteHeader(http.StatusOK)
		return true
	}
	return false
}

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

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(creds.Password), BcryptCost)
	if err != nil {
		http.Error(w, "Sunucu hatası", http.StatusInternalServerError)
		return
	}

	var userID int
	err = db.QueryRow("INSERT INTO users (username, password_hash, email, is_guest) VALUES ($1, $2, $3, FALSE) RETURNING id",
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

	tokenString, err := issueJWTForUser(userID, 7)
	if err != nil {
		http.Error(w, "Token oluşturulamadı", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"token": tokenString, "username": realUsername})
}

type ProfileData struct {
	Username        string   `json:"username"`
	Avatar          string   `json:"avatar"`
	Country         string   `json:"country"`
	XP              int      `json:"xp"`
	Tokens          int      `json:"tokens"`
	Cases           int      `json:"cases"`
	Wins            int      `json:"wins"`
	Games           int      `json:"gamesPlayed"`
	HighScore       int      `json:"highScore"`
	UnlockedDice    []string `json:"unlockedDice"`
	SelectedDice    string   `json:"selectedDice"`
	LoginStreak     int      `json:"loginStreak"`
	LastDailyReward string   `json:"lastDailyReward"`
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
	if uid, err := strconv.Atoi(claims.Subject); err == nil {
		touchUserPresence(uid)
	}
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
	var unlockedStr string
	err := db.QueryRow(`
		SELECT u.username, COALESCE(p.avatar, ''), COALESCE(p.country, 'TR'),
		       p.xp, p.tokens, p.cases, p.wins, p.games_played, p.high_score,
		       COALESCE(p.unlocked_dice, 'ivory'), COALESCE(p.selected_dice, 'ivory'),
		       COALESCE(p.login_streak, 0), COALESCE(p.last_daily_reward, '')
		FROM profiles p
		JOIN users u ON u.id = p.user_id
		WHERE p.user_id = $1
	`, userID).Scan(
		&p.Username, &p.Avatar, &p.Country,
		&p.XP, &p.Tokens, &p.Cases, &p.Wins, &p.Games, &p.HighScore,
		&unlockedStr, &p.SelectedDice, &p.LoginStreak, &p.LastDailyReward,
	)
	if err != nil {
		return p, err
	}
	if unlockedStr == "" {
		p.UnlockedDice = []string{"ivory"}
	} else {
		p.UnlockedDice = strings.Split(unlockedStr, ",")
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
	http.HandleFunc("/auth/forgot-password", handleForgotPassword)
	http.HandleFunc("/auth/reset-password", handleResetPassword)
	http.HandleFunc("/profile", handleGetProfile)
	http.HandleFunc("/profile/update", handleUpdateProfile)
	http.HandleFunc("/profile/claim-daily", handleClaimDaily)
	http.HandleFunc("/profile/open-case", handleOpenCase)
	http.HandleFunc("/shop/buy", handleBuyDice)
	http.HandleFunc("/friends", handleGetFriends)
	http.HandleFunc("/friends/request", handleFriendRequest)
	http.HandleFunc("/friends/respond", handleFriendRespond)
	http.HandleFunc("/friends/invite", handleFriendInvite)
	http.HandleFunc("/play", handleConnections)

	port := getEnv("PORT", "8080")
	fmt.Printf("🚀 Sunucu %s portunda aktif...\n", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}
