package main

import (
	"bufio"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestValidEmailAddress(t *testing.T) {
	for address, want := range map[string]bool{
		"ada@example.com":        true,
		"not-an-email":           false,
		"Ada <ada@example.com>":  false,
		"ada@example.com\r\nBcc": false,
	} {
		if got := validEmailAddress(address); got != want {
			t.Errorf("%q: got %v, want %v", address, got, want)
		}
	}
}

// Tek mesaj alıp DATA içeriğini döndüren küçük SMTP sunucusu
func startFakeSMTP(t *testing.T) (string, <-chan string) {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	received := make(chan string, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		reader := bufio.NewReader(conn)
		fmt.Fprint(conn, "220 test\r\n")
		inData := false
		var body strings.Builder
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				return
			}
			if inData {
				if line == ".\r\n" {
					inData = false
					received <- body.String()
					fmt.Fprint(conn, "250 ok\r\n")
				} else {
					body.WriteString(line)
				}
				continue
			}
			switch {
			case strings.HasPrefix(line, "DATA"):
				inData = true
				fmt.Fprint(conn, "354 go\r\n")
			case strings.HasPrefix(line, "QUIT"):
				fmt.Fprint(conn, "221 bye\r\n")
				return
			default:
				fmt.Fprint(conn, "250 ok\r\n")
			}
		}
	}()
	return listener.Addr().String(), received
}

func TestSendEmail(t *testing.T) {
	addr, received := startFakeSMTP(t)
	host, port, _ := net.SplitHostPort(addr)
	t.Setenv("SMTP_HOST", host)
	t.Setenv("SMTP_PORT", port)
	t.Setenv("SMTP_FROM", "no-reply@example.com")

	if err := sendEmail("ada@example.com\r\nBcc: x@example.com", "Konu", "Gövde"); err == nil {
		t.Fatal("header injection was not rejected")
	}
	if err := sendEmail("ada@example.com", "Şifre kodu", "Kodunuz: 123456"); err != nil {
		t.Fatal(err)
	}
	select {
	case message := <-received:
		if !strings.Contains(message, "To: ada@example.com") || !strings.Contains(message, "Kodunuz: 123456") {
			t.Fatalf("unexpected message: %s", message)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("message was not delivered")
	}
}

func TestAdminRequiresStrongToken(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/admin/queue", nil)
	request.Header.Set("X-Admin-Token", "short")

	t.Setenv("ADMIN_TOKEN", "short")
	if adminAuthorized(request) {
		t.Fatal("weak admin token was accepted")
	}
	strong := strings.Repeat("a", minAdminTokenLength)
	t.Setenv("ADMIN_TOKEN", strong)
	if adminAuthorized(request) {
		t.Fatal("wrong admin token was accepted")
	}
	request.Header.Set("X-Admin-Token", strong)
	if !adminAuthorized(request) {
		t.Fatal("correct admin token was rejected")
	}
}

func TestChatEvidenceKeepsLatestMessages(t *testing.T) {
	const userID = 987654
	t.Cleanup(func() {
		chatRecordsMu.Lock()
		delete(chatRecords, userID)
		chatRecordsMu.Unlock()
	})
	for i := 0; i < maxChatRecords+5; i++ {
		recordChatMessage(userID, fmt.Sprintf("mesaj-%d", i))
	}
	evidence := recentChatEvidence(userID)
	if strings.Contains(evidence, `"mesaj-0"`) || !strings.Contains(evidence, fmt.Sprintf(`"mesaj-%d"`, maxChatRecords+4)) {
		t.Fatalf("unexpected evidence: %s", evidence)
	}
}

func insertTestUser(t *testing.T, name string) int {
	t.Helper()
	var id int
	if err := db.QueryRow(`INSERT INTO users (username, password_hash) VALUES ($1, 'x') RETURNING id`, name).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func TestBlocksWorkInBothDirections(t *testing.T) {
	openTestDB(t)
	suffix := time.Now().UnixNano()
	a := insertTestUser(t, fmt.Sprintf("blockA%d", suffix))
	b := insertTestUser(t, fmt.Sprintf("blockB%d", suffix))
	if isBlockedEither(a, b) {
		t.Fatal("users should start unblocked")
	}
	if _, err := db.Exec(`INSERT INTO user_blocks (blocker_id, blocked_id, blocked_username) VALUES ($1, $2, 'x')`, a, b); err != nil {
		t.Fatal(err)
	}
	if !isBlockedEither(a, b) || !isBlockedEither(b, a) {
		t.Fatal("block must apply in both directions")
	}
}

func TestDeleteUserDataRemovesAccount(t *testing.T) {
	openTestDB(t)
	suffix := time.Now().UnixNano()
	reporter := insertTestUser(t, fmt.Sprintf("delA%d", suffix))
	target := insertTestUser(t, fmt.Sprintf("delB%d", suffix))
	if _, err := db.Exec(`INSERT INTO user_reports (reporter_id, reported_user_id, reported_username, reason) VALUES ($1, $2, 'x', 'test')`, reporter, target); err != nil {
		t.Fatal(err)
	}
	if err := deleteUserData(reporter); err != nil {
		t.Fatal(err)
	}
	var users, namedReports int
	_ = db.QueryRow(`SELECT COUNT(*) FROM users WHERE id = $1`, reporter).Scan(&users)
	_ = db.QueryRow(`SELECT COUNT(*) FROM user_reports WHERE reporter_id = $1`, reporter).Scan(&namedReports)
	if users != 0 || namedReports != 0 {
		t.Fatalf("user rows: %d, reports naming the reporter: %d", users, namedReports)
	}
}

func TestWebDeletionRequestDoesNotRevealAccount(t *testing.T) {
	openTestDB(t)
	form := url.Values{"username": {fmt.Sprintf("yok%d", time.Now().UnixNano())}, "email": {"a@example.com"}, "confirm": {"yes"}}
	request := httptest.NewRequest(http.MethodPost, "/account/delete", strings.NewReader(form.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	handleWebAccountDeletion(recorder, request)
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), "Talebiniz Alındı") {
		t.Fatalf("got %d: %s", recorder.Code, recorder.Body)
	}
}

func TestAdminClearsReportedAvatar(t *testing.T) {
	openTestDB(t)
	token := strings.Repeat("b", minAdminTokenLength)
	t.Setenv("ADMIN_TOKEN", token)
	suffix := time.Now().UnixNano()
	reporter := insertTestUser(t, fmt.Sprintf("avA%d", suffix))
	target := insertTestUser(t, fmt.Sprintf("avB%d", suffix))
	const avatar = "data:image/png;base64,AAAA"
	if _, err := db.Exec(`INSERT INTO profiles (user_id, avatar) VALUES ($1, $2) ON CONFLICT (user_id) DO UPDATE SET avatar = $2`, target, avatar); err != nil {
		t.Fatal(err)
	}
	var reportID int
	if err := db.QueryRow(`INSERT INTO user_reports (reporter_id, reported_user_id, reported_username, reason) VALUES ($1, $2, 'x', 'foto') RETURNING id`, reporter, target).Scan(&reportID); err != nil {
		t.Fatal(err)
	}

	queue := httptest.NewRequest(http.MethodGet, "/admin/queue", nil)
	queue.Header.Set("X-Admin-Token", token)
	recorder := httptest.NewRecorder()
	handleAdminQueue(recorder, queue)
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), avatar) {
		t.Fatalf("queue %d does not show the avatar: %s", recorder.Code, recorder.Body)
	}

	body := fmt.Sprintf(`{"kind":"report","id":%d,"action":"clear_avatar"}`, reportID)
	action := httptest.NewRequest(http.MethodPost, "/admin/action", strings.NewReader(body))
	action.Header.Set("X-Admin-Token", token)
	recorder = httptest.NewRecorder()
	handleAdminAction(recorder, action)
	if recorder.Code != http.StatusOK {
		t.Fatalf("clear_avatar failed %d: %s", recorder.Code, recorder.Body)
	}
	var stored, status string
	_ = db.QueryRow(`SELECT avatar FROM profiles WHERE user_id = $1`, target).Scan(&stored)
	_ = db.QueryRow(`SELECT status FROM user_reports WHERE id = $1`, reportID).Scan(&status)
	if stored != "" || status != "content_removed" {
		t.Fatalf("avatar %q, report status %q", stored, status)
	}
	if isUserBanned(target) {
		t.Fatal("removing a photo must not ban the account")
	}
}

func TestSupportPageShowsContact(t *testing.T) {
	t.Setenv("LEGAL_CONTACT_EMAIL", "destek@example.com")
	recorder := httptest.NewRecorder()
	handleSupportHTML(recorder, httptest.NewRequest(http.MethodGet, "/support", nil))
	if !strings.Contains(recorder.Body.String(), "mailto:destek@example.com") {
		t.Fatalf("support page lacks contact: %s", recorder.Body)
	}
}

func TestSupportEmailDefault(t *testing.T) {
	t.Setenv("LEGAL_CONTACT_EMAIL", "")
	recorder := httptest.NewRecorder()
	handlePrivacyHTML(recorder, httptest.NewRequest(http.MethodGet, "/privacy", nil))
	if !strings.Contains(recorder.Body.String(), "mailto:yatdice@gmail.com") {
		t.Fatal("privacy page does not show the default support email")
	}
}
