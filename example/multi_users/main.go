package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/smtp"
	"os"
	"strings"
	"time"
)

type userResponse struct {
	Email  string   `json:"email"`
	Emails []string `json:"emails"`
}

type accountsResponse struct {
	Accounts []struct {
		Email  string `json:"email"`
		Unread int    `json:"unread"`
	} `json:"accounts"`
}

type messagesResponse struct {
	Messages []struct {
		ID string `json:"id"`
	} `json:"messages"`
	Total int `json:"total"`
}

func main() {
	baseURL := getenvDefault("LOCALSMTP_URL", "http://localhost:3025")
	smtpAddr := getenvDefault("LOCALSMTP_SMTP", "localhost:2025")
	smtpUser := os.Getenv("SMTP_USERNAME")
	smtpPass := os.Getenv("SMTP_PASSWORD")

	client := newClient()

	userA := "test1@localsmtp.dev"
	userB := "test2@localsmtp.dev"

	fmt.Println("Logging in as", userA)
	loginUser(client, baseURL, userA)
	fmt.Println("Logging in as", userB)
	loginUser(client, baseURL, userB)

	fmt.Println("Accounts:")
	accounts := getAccounts(client, baseURL)
	for _, account := range accounts.Accounts {
		fmt.Printf("- %s unread=%d\n", account.Email, account.Unread)
	}

	fmt.Println("Sending test emails...")
	sendSMTP(
		smtpAddr,
		smtpUser,
		smtpPass,
		"sender@localsmtp.dev",
		[]string{userA},
		buildTestMessage("Test 1 - HTML + Text", userA),
	)
	sendSMTP(
		smtpAddr,
		smtpUser,
		smtpPass,
		"sender@localsmtp.dev",
		[]string{userB},
		buildTestMessage("Test 2 - HTML + Text", userB),
	)
	sendSMTP(
		smtpAddr,
		smtpUser,
		smtpPass,
		"sender@localsmtp.dev",
		[]string{userA, userB},
		buildTestMessage("Test 3 - Multi-recipient", userA+", "+userB),
	)

	time.Sleep(500 * time.Millisecond)

	fmt.Println("Accounts after send:")
	accounts = getAccounts(client, baseURL)
	for _, account := range accounts.Accounts {
		fmt.Printf("- %s unread=%d\n", account.Email, account.Unread)
	}

	fmt.Println("Listing messages per account:")
	for _, email := range []string{userA, userB} {
		resp := listMessages(client, baseURL, email)
		fmt.Printf("- %s total=%d\n", email, resp.Total)
	}
}

func newClient() *http.Client {
	jar, _ := cookiejar.New(nil)
	return &http.Client{
		Timeout: 10 * time.Second,
		Jar:     jar,
	}
}

func loginUser(client *http.Client, baseURL, email string) userResponse {
	payload, _ := json.Marshal(map[string]string{"email": email})
	resp := mustDo(client, "POST", baseURL+"/api/login", bytes.NewReader(payload))
	defer resp.Body.Close()
	var out userResponse
	mustDecode(resp.Body, &out)
	return out
}

func getAccounts(client *http.Client, baseURL string) accountsResponse {
	resp := mustDo(client, "GET", baseURL+"/api/accounts", nil)
	defer resp.Body.Close()
	var out accountsResponse
	mustDecode(resp.Body, &out)
	return out
}

func listMessages(client *http.Client, baseURL, email string) messagesResponse {
	url := fmt.Sprintf("%s/api/messages?email=%s&box=inbox&page=1&limit=5", baseURL, email)
	resp := mustDo(client, "GET", url, nil)
	defer resp.Body.Close()
	var out messagesResponse
	mustDecode(resp.Body, &out)
	return out
}

func sendSMTP(addr, username, password, from string, to []string, msg []byte) {
	var auth smtp.Auth
	if username != "" || password != "" {
		host, _, err := net.SplitHostPort(addr)
		if err != nil {
			host = addr
		}
		auth = smtp.PlainAuth("", username, password, host)
	}
	if err := smtp.SendMail(addr, auth, from, to, msg); err != nil {
		fmt.Fprintln(os.Stderr, "smtp error:", err)
	}
}

func buildTestMessage(subject, recipients string) []byte {
	boundary := fmt.Sprintf("localsmtp-%d", time.Now().UnixNano())
	text := "Hello!\n\nThis is a LocalSMTP multi-account test email.\n\nRecipients: " + recipients + "\n"
	html := "<html><body><h2>LocalSMTP multi-account test</h2><p>This is a test email.</p><p><strong>Recipients:</strong> " + recipients + "</p></body></html>"
	headers := []string{
		"From: sender@localsmtp.dev",
		"To: " + recipients,
		"Subject: " + subject,
		"Date: " + time.Now().Format(time.RFC1123Z),
		"MIME-Version: 1.0",
		"Content-Type: multipart/alternative; boundary=" + boundary,
		"",
		"--" + boundary,
		"Content-Type: text/plain; charset=utf-8",
		"",
		text,
		"--" + boundary,
		"Content-Type: text/html; charset=utf-8",
		"",
		html,
		"--" + boundary + "--",
		"",
	}
	return []byte(strings.Join(headers, "\r\n"))
}

func mustDo(client *http.Client, method, url string, body io.Reader) *http.Response {
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		panic(err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := client.Do(req)
	if err != nil {
		panic(err)
	}
	if resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		panic(fmt.Sprintf("request failed: %s %s: %s", method, url, string(b)))
	}
	return resp
}

func mustDecode(r io.Reader, v any) {
	dec := json.NewDecoder(r)
	if err := dec.Decode(v); err != nil {
		panic(err)
	}
}

func getenvDefault(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
