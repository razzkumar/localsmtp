package smtpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"time"

	"github.com/emersion/go-message/mail"
	"github.com/emersion/go-sasl"
	"github.com/emersion/go-smtp"
	"github.com/google/uuid"

	"github.io/razzkumar/localsmtp/internal/sse"
	"github.io/razzkumar/localsmtp/internal/store"
)

const (
	defaultDomain = "localsmtp"
)

type AuthConfig struct {
	Enabled  bool
	Username string
	Password string
}

type Server struct {
	smtp   *smtp.Server
	logger *slog.Logger
}

func New(store *store.Store, hub *sse.Hub, logger *slog.Logger, addr string, authCfg AuthConfig) *Server {
	backend := &backend{
		store:        store,
		hub:          hub,
		logger:       logger,
		authEnabled:  authCfg.Enabled,
		authUsername: authCfg.Username,
		authPassword: authCfg.Password,
	}
	server := smtp.NewServer(backend)
	server.Addr = addr
	server.Domain = defaultDomain
	server.AllowInsecureAuth = true
	server.ReadTimeout = 15 * time.Second
	server.WriteTimeout = 15 * time.Second
	server.MaxRecipients = 100
	server.MaxMessageBytes = 25 << 20

	return &Server{smtp: server, logger: logger}
}

func (s *Server) ListenAndServe() error {
	s.logger.Info("smtp server listening", "addr", s.smtp.Addr)
	return s.smtp.ListenAndServe()
}

func (s *Server) Close() error {
	return s.smtp.Close()
}

type backend struct {
	store        *store.Store
	hub          *sse.Hub
	logger       *slog.Logger
	authEnabled  bool
	authUsername string
	authPassword string
}

func (b *backend) NewSession(_ *smtp.Conn) (smtp.Session, error) {
	return &session{backend: b}, nil
}

type session struct {
	backend       *backend
	from          string
	to            []string
	authenticated bool
}

func (s *session) AuthMechanisms() []string {
	if s.backend.authEnabled {
		return []string{sasl.Plain}
	}
	return nil
}

func (s *session) Auth(mech string) (sasl.Server, error) {
	if !s.backend.authEnabled {
		return nil, errors.New("authentication not enabled")
	}
	if mech != sasl.Plain {
		return nil, errors.New("unsupported authentication mechanism")
	}
	return sasl.NewPlainServer(func(identity, username, password string) error {
		if username == s.backend.authUsername && password == s.backend.authPassword {
			s.authenticated = true
			return nil
		}
		return errors.New("invalid credentials")
	}), nil
}

func (s *session) Mail(from string, _ *smtp.MailOptions) error {
	if s.backend.authEnabled && !s.authenticated {
		return smtp.ErrAuthRequired
	}
	s.from = normalizeEmail(from)
	return nil
}

func (s *session) Rcpt(to string, _ *smtp.RcptOptions) error {
	if s.backend.authEnabled && !s.authenticated {
		return smtp.ErrAuthRequired
	}
	s.to = append(s.to, normalizeEmail(to))
	return nil
}

func (s *session) Data(r io.Reader) error {
	data, err := io.ReadAll(r)
	if err != nil {
		return err
	}

	message, recipients, attachments, err := parseMessage(s.from, s.to, data)
	if err != nil {
		s.backend.logger.Warn("parse smtp message", "error", err)
	}

	ctx := context.Background()
	if err := s.backend.store.InsertMessage(ctx, message, recipients, attachments); err != nil {
		s.backend.logger.Error("store smtp message", "error", err)
		return err
	}

	s.backend.hub.Broadcast(messageAudience(message, recipients), buildEvent(message, recipients))
	return nil
}

func (s *session) Reset() {
	s.from = ""
	s.to = nil
}

func (s *session) Logout() error {
	return nil
}

func parseMessage(envelopeFrom string, envelopeTo []string, raw []byte) (store.Message, []store.Recipient, []store.Attachment, error) {
	message := store.Message{
		ID:        uuid.NewString(),
		From:      normalizeEmail(envelopeFrom),
		Subject:   "",
		TextBody:  "",
		HTMLBody:  "",
		Raw:       raw,
		RawSize:   int64(len(raw)),
		CreatedAt: time.Now(),
	}

	recipients := map[string]map[string]struct{}{}
	attachments := []store.Attachment{}

	reader, err := mail.CreateReader(bytes.NewReader(raw))
	if err != nil {
		return message, recipientsFromEnvelope(envelopeTo, recipients), attachments, err
	}

	if subject, err := reader.Header.Subject(); err == nil {
		message.Subject = subject
	}

	if fromList, err := reader.Header.AddressList("From"); err == nil && len(fromList) > 0 {
		if message.From == "" {
			message.From = normalizeEmail(fromList[0].Address)
		}
	}
	if message.From == "" {
		message.From = "unknown@localsmtp"
	}

	addHeaderRecipients := func(headerName, rtype string) {
		if list, err := reader.Header.AddressList(headerName); err == nil {
			for _, addr := range list {
				addRecipient(recipients, rtype, normalizeEmail(addr.Address))
			}
		}
	}
	addHeaderRecipients("To", "to")
	addHeaderRecipients("Cc", "cc")
	addHeaderRecipients("Bcc", "bcc")

	for {
		part, err := reader.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			return message, recipientsFromEnvelope(envelopeTo, recipients), attachments, err
		}

		switch header := part.Header.(type) {
		case *mail.InlineHeader:
			mediaType, _, _ := header.ContentType()
			body, err := io.ReadAll(part.Body)
			if err != nil {
				continue
			}
			switch {
			case strings.HasPrefix(mediaType, "text/plain") || mediaType == "":
				if message.TextBody == "" {
					message.TextBody = string(body)
				} else {
					message.TextBody += "\n" + string(body)
				}
			case strings.HasPrefix(mediaType, "text/html"):
				if message.HTMLBody == "" {
					message.HTMLBody = string(body)
				} else {
					message.HTMLBody += "\n" + string(body)
				}
			}
		case *mail.AttachmentHeader:
			filename, _ := header.Filename()
			if strings.TrimSpace(filename) == "" {
				filename = "attachment"
			}
			contentType, _, _ := header.ContentType()
			body, err := io.ReadAll(part.Body)
			if err != nil {
				continue
			}
			attachments = append(attachments, store.Attachment{
				Filename:    filename,
				ContentType: contentType,
				Data:        body,
				Size:        int64(len(body)),
			})
		}
	}

	return message, recipientsFromEnvelope(envelopeTo, recipients), attachments, nil
}

func recipientsFromEnvelope(envelopeTo []string, base map[string]map[string]struct{}) []store.Recipient {
	for _, addr := range envelopeTo {
		addRecipient(base, "to", normalizeEmail(addr))
	}
	return flattenRecipients(base)
}

func addRecipient(base map[string]map[string]struct{}, rtype, email string) {
	if email == "" {
		return
	}
	if _, ok := base[rtype]; !ok {
		base[rtype] = map[string]struct{}{}
	}
	base[rtype][email] = struct{}{}
}

func flattenRecipients(base map[string]map[string]struct{}) []store.Recipient {
	var recipients []store.Recipient
	for rtype, emails := range base {
		for email := range emails {
			recipients = append(recipients, store.Recipient{Email: email, Type: rtype})
		}
	}
	return recipients
}

func normalizeEmail(email string) string {
	return strings.TrimSpace(strings.ToLower(email))
}

func messageAudience(message store.Message, recipients []store.Recipient) []string {
	audience := []string{message.From}
	for _, recipient := range recipients {
		audience = append(audience, recipient.Email)
	}
	return audience
}

func buildEvent(message store.Message, recipients []store.Recipient) []byte {
	toList := []string{}
	for _, recipient := range recipients {
		if recipient.Type == "to" {
			toList = append(toList, recipient.Email)
		}
	}
	payload := map[string]any{
		"id":        message.ID,
		"from":      message.From,
		"to":        toList,
		"createdAt": message.CreatedAt.UTC().Format(time.RFC3339),
	}
	data, _ := json.Marshal(payload)
	return []byte(fmt.Sprintf("event: message\ndata: %s\n\n", data))
}
