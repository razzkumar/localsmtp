package api

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"net/smtp"
	"path"
	"strconv"
	"strings"
	"time"

	"github.io/infrasutra/localsmtp/internal/auth"
	"github.io/infrasutra/localsmtp/internal/config"
	"github.io/infrasutra/localsmtp/internal/sse"
	"github.io/infrasutra/localsmtp/internal/store"
	webassets "github.io/infrasutra/localsmtp/web"
)

type Server struct {
	cfg      config.Config
	store    *store.Store
	auth     *auth.Manager
	hub      *sse.Hub
	logger   *slog.Logger
	smtpAddr string
	mux      *http.ServeMux
	staticFS fs.FS
	staticOK bool
}

func NewServer(cfg config.Config, store *store.Store, authManager *auth.Manager, hub *sse.Hub, logger *slog.Logger) *Server {
	staticFS, err := webassets.Dist()
	staticOK := err == nil
	if err != nil {
		logger.Warn("ui assets not embedded", "error", err)
	}
	server := &Server{
		cfg:      cfg,
		store:    store,
		auth:     authManager,
		hub:      hub,
		logger:   logger,
		smtpAddr: fmt.Sprintf("127.0.0.1:%d", cfg.SMTPPort),
		staticFS: staticFS,
		staticOK: staticOK,
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/login", server.handleLogin)
	mux.HandleFunc("/api/logout", server.handleLogout)
	mux.HandleFunc("/api/me", server.handleMe)
	mux.HandleFunc("/api/messages", server.handleMessages)
	mux.HandleFunc("/api/messages/", server.handleMessage)
	mux.HandleFunc("/api/stream", server.handleStream)
	mux.HandleFunc("/api/send", server.handleSend)
	server.mux = mux
	return server
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	if strings.HasPrefix(path, "/api/") {
		s.mux.ServeHTTP(w, r)
		return
	}
	if strings.HasPrefix(path, "/webhooks") {
		http.NotFound(w, r)
		return
	}
	if path == "/health" {
		s.handleHealth(w, r)
		return
	}
	if path == "/ready" {
		s.handleReady(w, r)
		return
	}
	if path == "/metrics" {
		s.handleMetrics(w, r)
		return
	}

	s.serveStatic(w, r)
}

func (s *Server) serveStatic(w http.ResponseWriter, r *http.Request) {
	if !s.staticOK {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte("UI not built. Run npm install && npm run build inside ./web."))
		return
	}

	cleaned := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
	if cleaned == "" {
		cleaned = "index.html"
	}

	if strings.HasPrefix(cleaned, "assets/") {
		if s.serveEmbeddedFile(w, r, cleaned) {
			return
		}
		http.NotFound(w, r)
		return
	}

	if s.serveEmbeddedFile(w, r, cleaned) {
		return
	}

	if s.serveEmbeddedFile(w, r, "index.html") {
		return
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusNotFound)
	_, _ = w.Write([]byte("UI not built. Run npm install && npm run build inside ./web."))
}

func (s *Server) serveEmbeddedFile(w http.ResponseWriter, r *http.Request, name string) bool {
	file, err := s.staticFS.Open(name)
	if err != nil {
		return false
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil || info.IsDir() {
		return false
	}

	if seeker, ok := file.(io.ReadSeeker); ok {
		http.ServeContent(w, r, info.Name(), info.ModTime(), seeker)
		return true
	}

	data, err := io.ReadAll(file)
	if err != nil {
		return false
	}
	reader := bytes.NewReader(data)
	http.ServeContent(w, r, info.Name(), info.ModTime(), reader)
	return true
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var payload struct {
		Email string `json:"email"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	email, err := auth.NormalizeEmail(payload.Email)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	now := time.Now()
	if err := s.store.UpsertUser(r.Context(), email, now); err != nil {
		http.Error(w, "unable to save user", http.StatusInternalServerError)
		return
	}
	token, err := s.auth.Issue(email, now)
	if err != nil {
		http.Error(w, "unable to create session", http.StatusInternalServerError)
		return
	}
	s.setSessionCookie(w, token, now)
	s.respondJSON(w, http.StatusOK, map[string]string{"email": email})
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     s.auth.CookieName(),
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	email, err := s.sessionEmail(r)
	if err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	s.respondJSON(w, http.StatusOK, map[string]string{"email": email})
}

func (s *Server) handleMessages(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	email, err := s.sessionEmail(r)
	if err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	box := r.URL.Query().Get("box")
	if box == "" {
		box = "inbox"
	}
	if box != "inbox" && box != "sent" {
		http.Error(w, "invalid box", http.StatusBadRequest)
		return
	}
	search := strings.TrimSpace(r.URL.Query().Get("search"))
	limit := 10
	if rawLimit := r.URL.Query().Get("limit"); rawLimit != "" {
		if parsed, err := strconv.Atoi(rawLimit); err == nil && parsed > 0 {
			if parsed > 100 {
				parsed = 100
			}
			limit = parsed
		}
	}
	cursor := strings.TrimSpace(r.URL.Query().Get("cursor"))
	messages, nextCursor, err := s.store.ListMessages(r.Context(), email, box, search, cursor, limit)
	if err != nil {
		if errors.Is(err, store.ErrInvalidCursor) {
			http.Error(w, "invalid cursor", http.StatusBadRequest)
			return
		}
		http.Error(w, "unable to list messages", http.StatusInternalServerError)
		return
	}

	response := struct {
		Messages   []messageSummary `json:"messages"`
		NextCursor string           `json:"nextCursor"`
		HasMore    bool             `json:"hasMore"`
	}{
		Messages:   make([]messageSummary, 0, len(messages)),
		NextCursor: nextCursor,
		HasMore:    nextCursor != "",
	}
	for _, msg := range messages {
		response.Messages = append(response.Messages, toSummary(msg))
	}
	s.respondJSON(w, http.StatusOK, response)
}

func (s *Server) handleMessage(w http.ResponseWriter, r *http.Request) {
	email, err := s.sessionEmail(r)
	if err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	path := strings.TrimPrefix(r.URL.Path, "/api/messages/")
	parts := strings.Split(path, "/")
	if len(parts) == 0 || parts[0] == "" {
		http.NotFound(w, r)
		return
	}
	id := parts[0]

	if len(parts) == 1 {
		switch r.Method {
		case http.MethodGet:
			s.handleMessageDetail(w, r, email, id)
		case http.MethodDelete:
			s.handleMessageDelete(w, r, email, id)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
		return
	}

	if len(parts) == 2 && parts[1] == "raw" {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		s.handleMessageRaw(w, r, email, id)
		return
	}

	if len(parts) == 3 && parts[1] == "attachments" {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		attachmentID, err := strconv.ParseInt(parts[2], 10, 64)
		if err != nil {
			http.Error(w, "invalid attachment id", http.StatusBadRequest)
			return
		}
		s.handleAttachment(w, r, email, attachmentID)
		return
	}

	http.NotFound(w, r)
}

func (s *Server) handleMessageDetail(w http.ResponseWriter, r *http.Request, email, id string) {
	message, recipients, attachments, err := s.store.GetMessage(r.Context(), email, id)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return
		}
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		http.Error(w, "unable to load message", http.StatusInternalServerError)
		return
	}

	detail := messageDetail{
		ID:          message.ID,
		From:        message.From,
		Subject:     message.Subject,
		Text:        message.TextBody,
		HTML:        message.HTMLBody,
		CreatedAt:   message.CreatedAt.UTC().Format(time.RFC3339),
		RawSize:     message.RawSize,
		To:          []string{},
		Cc:          []string{},
		Bcc:         []string{},
		Attachments: []attachmentSummary{},
	}
	for _, recipient := range recipients {
		switch recipient.Type {
		case "cc":
			detail.Cc = append(detail.Cc, recipient.Email)
		case "bcc":
			detail.Bcc = append(detail.Bcc, recipient.Email)
		default:
			detail.To = append(detail.To, recipient.Email)
		}
	}
	for _, attachment := range attachments {
		detail.Attachments = append(detail.Attachments, attachmentSummary{
			ID:          attachment.ID,
			Filename:    attachment.Filename,
			ContentType: attachment.ContentType,
			Size:        attachment.Size,
		})
	}
	s.respondJSON(w, http.StatusOK, detail)
}

func (s *Server) handleMessageRaw(w http.ResponseWriter, r *http.Request, email, id string) {
	message, _, _, err := s.store.GetMessage(r.Context(), email, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		http.Error(w, "unable to load message", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "message/rfc822")
	w.Header().Set("Content-Disposition", fmt.Sprintf("inline; filename=message-%s.eml", message.ID))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(message.Raw)
}

func (s *Server) handleAttachment(w http.ResponseWriter, r *http.Request, email string, attachmentID int64) {
	attachment, err := s.store.GetAttachment(r.Context(), email, attachmentID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		http.Error(w, "unable to load attachment", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", attachment.ContentType)
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", attachment.Filename))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(attachment.Data)
}

func (s *Server) handleMessageDelete(w http.ResponseWriter, r *http.Request, email, id string) {
	deleted, err := s.store.DeleteMessage(r.Context(), email, id)
	if err != nil {
		http.Error(w, "unable to delete", http.StatusInternalServerError)
		return
	}
	if !deleted {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleStream(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	email, err := s.sessionEmail(r)
	if err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	ch, unsubscribe := s.hub.Subscribe(email)
	defer unsubscribe()

	_, _ = w.Write([]byte("event: ready\ndata: {}\n\n"))
	flusher.Flush()

	ticker := time.NewTicker(20 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case payload, ok := <-ch:
			if !ok {
				return
			}
			_, _ = w.Write(payload)
			flusher.Flush()
		case <-ticker.C:
			_, _ = w.Write([]byte(": ping\n\n"))
			flusher.Flush()
		}
	}
}

func (s *Server) handleSend(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	email, err := s.sessionEmail(r)
	if err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	var payload sendRequest
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	recipients := normalizeRecipients(payload.To)
	if len(recipients) == 0 {
		http.Error(w, "at least one recipient required", http.StatusBadRequest)
		return
	}
	subject := strings.TrimSpace(payload.Subject)
	textBody := strings.TrimSpace(payload.Text)
	htmlBody := strings.TrimSpace(payload.HTML)
	if textBody == "" && htmlBody == "" {
		http.Error(w, "message body required", http.StatusBadRequest)
		return
	}

	raw := buildOutboundMessage(email, recipients, subject, textBody, htmlBody)
	if err := smtp.SendMail(s.smtpAddr, nil, email, recipients, raw); err != nil {
		s.logger.Error("send mail", "error", err)
		http.Error(w, "unable to send mail", http.StatusBadRequest)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) sessionEmail(r *http.Request) (string, error) {
	cookie, err := r.Cookie(s.auth.CookieName())
	if err != nil {
		return "", errors.New("missing session")
	}
	email, err := s.auth.Parse(cookie.Value, time.Now())
	if err != nil {
		return "", err
	}
	return email, nil
}

func (s *Server) setSessionCookie(w http.ResponseWriter, value string, now time.Time) {
	maxAge := int(s.auth.MaxAge().Seconds())
	http.SetCookie(w, &http.Cookie{
		Name:     s.auth.CookieName(),
		Value:    value,
		Path:     "/",
		MaxAge:   maxAge,
		Expires:  now.Add(s.auth.MaxAge()),
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}

func (s *Server) respondJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	s.respondText(w, http.StatusOK, "ok")
}

func (s *Server) handleReady(w http.ResponseWriter, _ *http.Request) {
	s.respondText(w, http.StatusOK, "ready")
}

func (s *Server) handleMetrics(w http.ResponseWriter, _ *http.Request) {
	s.respondText(w, http.StatusOK, "metrics")
}

func (s *Server) respondText(w http.ResponseWriter, status int, payload string) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(payload))
}

type messageSummary struct {
	ID             string   `json:"id"`
	From           string   `json:"from"`
	To             []string `json:"to"`
	Subject        string   `json:"subject"`
	CreatedAt      string   `json:"createdAt"`
	HasAttachments bool     `json:"hasAttachments"`
}

type messageDetail struct {
	ID          string              `json:"id"`
	From        string              `json:"from"`
	To          []string            `json:"to"`
	Cc          []string            `json:"cc"`
	Bcc         []string            `json:"bcc"`
	Subject     string              `json:"subject"`
	Text        string              `json:"text"`
	HTML        string              `json:"html"`
	CreatedAt   string              `json:"createdAt"`
	RawSize     int64               `json:"rawSize"`
	Attachments []attachmentSummary `json:"attachments"`
}

type attachmentSummary struct {
	ID          int64  `json:"id"`
	Filename    string `json:"filename"`
	ContentType string `json:"contentType"`
	Size        int64  `json:"size"`
}

type sendRequest struct {
	To      []string `json:"to"`
	Subject string   `json:"subject"`
	Text    string   `json:"text"`
	HTML    string   `json:"html"`
}

func toSummary(msg store.MessageSummary) messageSummary {
	toList := []string{}
	if msg.RecipientGroups != nil {
		toList = append(toList, msg.RecipientGroups["to"]...)
	}
	return messageSummary{
		ID:             msg.ID,
		From:           msg.From,
		To:             toList,
		Subject:        msg.Subject,
		CreatedAt:      msg.CreatedAt.UTC().Format(time.RFC3339),
		HasAttachments: msg.HasAttachments,
	}
}

func normalizeRecipients(recipients []string) []string {
	seen := map[string]struct{}{}
	result := []string{}
	for _, recipient := range recipients {
		trimmed := strings.ToLower(strings.TrimSpace(recipient))
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		result = append(result, trimmed)
	}
	return result
}

func buildOutboundMessage(from string, to []string, subject, textBody, htmlBody string) []byte {
	boundary := fmt.Sprintf("localsmtp-%d", time.Now().UnixNano())
	from = sanitizeHeader(from)
	subject = sanitizeHeader(subject)
	cleanTo := make([]string, 0, len(to))
	for _, recipient := range to {
		cleanTo = append(cleanTo, sanitizeHeader(recipient))
	}
	headers := []string{
		fmt.Sprintf("From: %s", from),
		fmt.Sprintf("To: %s", strings.Join(cleanTo, ", ")),
		fmt.Sprintf("Subject: %s", subject),
		fmt.Sprintf("Date: %s", time.Now().Format(time.RFC1123Z)),
		"MIME-Version: 1.0",
	}

	if textBody != "" && htmlBody != "" {
		headers = append(headers, fmt.Sprintf("Content-Type: multipart/alternative; boundary=%q", boundary))
		body := []string{
			"",
			fmt.Sprintf("--%s", boundary),
			"Content-Type: text/plain; charset=utf-8",
			"",
			textBody,
			fmt.Sprintf("--%s", boundary),
			"Content-Type: text/html; charset=utf-8",
			"",
			htmlBody,
			fmt.Sprintf("--%s--", boundary),
			"",
		}
		return []byte(strings.Join(append(headers, body...), "\r\n"))
	}

	contentType := "text/plain"
	body := textBody
	if body == "" {
		contentType = "text/html"
		body = htmlBody
	}
	headers = append(headers, fmt.Sprintf("Content-Type: %s; charset=utf-8", contentType))
	return []byte(strings.Join(append(headers, "", body, ""), "\r\n"))
}

func sanitizeHeader(value string) string {
	cleaned := strings.ReplaceAll(value, "\r", "")
	cleaned = strings.ReplaceAll(cleaned, "\n", "")
	return strings.TrimSpace(cleaned)
}
