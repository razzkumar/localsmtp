package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"net/mail"
	"strconv"
	"strings"
	"time"
)

const (
	cookieName = "localsmtp_session"
)

type Manager struct {
	secret []byte
	maxAge time.Duration
}

func New(secret string, maxAge time.Duration) (*Manager, error) {
	if strings.TrimSpace(secret) == "" {
		generated := make([]byte, 32)
		if _, err := rand.Read(generated); err != nil {
			return nil, fmt.Errorf("generate auth secret: %w", err)
		}
		secret = base64.RawURLEncoding.EncodeToString(generated)
	}
	return &Manager{secret: []byte(secret), maxAge: maxAge}, nil
}

func (m *Manager) CookieName() string {
	return cookieName
}

func (m *Manager) MaxAge() time.Duration {
	return m.maxAge
}

func (m *Manager) Issue(email string, now time.Time) (string, error) {
	return m.IssueEmails([]string{email}, now)
}

func (m *Manager) IssueEmails(emails []string, now time.Time) (string, error) {
	normalized, err := normalizeEmailList(emails)
	if err != nil {
		return "", err
	}
	timestamp := strconv.FormatInt(now.Unix(), 10)
	payload := strings.Join(normalized, ",") + "|" + timestamp
	sig := m.sign(payload)
	token := payload + "|" + sig
	return base64.RawURLEncoding.EncodeToString([]byte(token)), nil
}

func (m *Manager) Parse(token string, now time.Time) ([]string, error) {
	if token == "" {
		return nil, errors.New("missing session token")
	}
	raw, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return nil, errors.New("invalid session token")
	}
	parts := strings.Split(string(raw), "|")
	if len(parts) != 3 {
		return nil, errors.New("invalid session token")
	}
	payload := parts[0] + "|" + parts[1]
	if !m.verify(payload, parts[2]) {
		return nil, errors.New("invalid session token")
	}
	timestamp, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return nil, errors.New("invalid session token")
	}
	issuedAt := time.Unix(timestamp, 0)
	if now.Sub(issuedAt) > m.maxAge {
		return nil, errors.New("session expired")
	}
	emails, err := normalizeEmailList(strings.Split(parts[0], ","))
	if err != nil {
		return nil, err
	}
	return emails, nil
}

func NormalizeEmail(email string) (string, error) {
	trimmed := strings.TrimSpace(strings.ToLower(email))
	if trimmed == "" {
		return "", errors.New("email is required")
	}
	addr, err := mail.ParseAddress(trimmed)
	if err != nil {
		return "", errors.New("email must be valid")
	}
	return strings.ToLower(addr.Address), nil
}

func normalizeEmailList(emails []string) ([]string, error) {
	seen := map[string]struct{}{}
	result := make([]string, 0, len(emails))
	for _, email := range emails {
		normalized, err := NormalizeEmail(email)
		if err != nil {
			return nil, err
		}
		if _, ok := seen[normalized]; ok {
			continue
		}
		seen[normalized] = struct{}{}
		result = append(result, normalized)
	}
	if len(result) == 0 {
		return nil, errors.New("email is required")
	}
	return result, nil
}

func (m *Manager) sign(payload string) string {
	mac := hmac.New(sha256.New, m.secret)
	mac.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func (m *Manager) verify(payload, signature string) bool {
	expected := m.sign(payload)
	return hmac.Equal([]byte(expected), []byte(signature))
}
