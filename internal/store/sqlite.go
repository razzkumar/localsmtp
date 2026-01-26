package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

type Store struct {
	db *sql.DB
}

func Open(ctx context.Context, path string) (*Store, error) {
	trimmed := strings.TrimSpace(path)
	inMemory := false
	if trimmed == "" {
		trimmed = ":memory:"
		inMemory = true
	}
	if strings.Contains(trimmed, "mode=memory") || trimmed == ":memory:" || trimmed == "file::memory:" {
		inMemory = true
	}
	db, err := sql.Open("sqlite", trimmed)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	if _, err := db.ExecContext(ctx, "PRAGMA foreign_keys = ON;"); err != nil {
		return nil, fmt.Errorf("enable foreign keys: %w", err)
	}
	if !inMemory {
		if _, err := db.ExecContext(ctx, "PRAGMA journal_mode = WAL;"); err != nil {
			return nil, fmt.Errorf("enable WAL: %w", err)
		}
	}
	return &Store{db: db}, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) EnsureSchema(ctx context.Context) error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS users (
            email TEXT PRIMARY KEY,
            created_at INTEGER NOT NULL,
            last_login INTEGER NOT NULL
        );`,
		`CREATE TABLE IF NOT EXISTS messages (
            id TEXT PRIMARY KEY,
            from_email TEXT NOT NULL,
            subject TEXT NOT NULL,
            text_body TEXT,
            html_body TEXT,
            raw BLOB NOT NULL,
            raw_size INTEGER NOT NULL,
            created_at INTEGER NOT NULL
        );`,
		`CREATE TABLE IF NOT EXISTS recipients (
            id INTEGER PRIMARY KEY AUTOINCREMENT,
            message_id TEXT NOT NULL,
            email TEXT NOT NULL,
            type TEXT NOT NULL,
            FOREIGN KEY(message_id) REFERENCES messages(id) ON DELETE CASCADE
        );`,
		`CREATE TABLE IF NOT EXISTS attachments (
            id INTEGER PRIMARY KEY AUTOINCREMENT,
            message_id TEXT NOT NULL,
            filename TEXT NOT NULL,
            content_type TEXT NOT NULL,
            data BLOB NOT NULL,
            size INTEGER NOT NULL,
            FOREIGN KEY(message_id) REFERENCES messages(id) ON DELETE CASCADE
        );`,
		`CREATE INDEX IF NOT EXISTS idx_recipients_email ON recipients(email);`,
		`CREATE INDEX IF NOT EXISTS idx_recipients_email_message ON recipients(email, message_id);`,
		`CREATE INDEX IF NOT EXISTS idx_recipients_message ON recipients(message_id);`,
		`CREATE INDEX IF NOT EXISTS idx_messages_from ON messages(from_email);`,
		`CREATE INDEX IF NOT EXISTS idx_messages_from_created ON messages(from_email, created_at);`,
		`CREATE INDEX IF NOT EXISTS idx_messages_created ON messages(created_at);`,
		`CREATE INDEX IF NOT EXISTS idx_messages_created_id ON messages(created_at, id);`,
	}

	for _, statement := range statements {
		if _, err := s.db.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("apply schema: %w", err)
		}
	}
	return nil
}

func (s *Store) UpsertUser(ctx context.Context, email string, now time.Time) error {
	query := `INSERT INTO users (email, created_at, last_login)
        VALUES (?, ?, ?)
        ON CONFLICT(email) DO UPDATE SET last_login = excluded.last_login;`
	_, err := s.db.ExecContext(ctx, query, email, now.Unix(), now.Unix())
	if err != nil {
		return fmt.Errorf("upsert user: %w", err)
	}
	return nil
}

func (s *Store) InsertMessage(ctx context.Context, message Message, recipients []Recipient, attachments []Attachment) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	_, err = tx.ExecContext(ctx, `INSERT INTO messages
        (id, from_email, subject, text_body, html_body, raw, raw_size, created_at)
        VALUES (?, ?, ?, ?, ?, ?, ?, ?);`,
		message.ID,
		message.From,
		message.Subject,
		message.TextBody,
		message.HTMLBody,
		message.Raw,
		message.RawSize,
		message.CreatedAt.Unix(),
	)
	if err != nil {
		return fmt.Errorf("insert message: %w", err)
	}

	for _, recipient := range recipients {
		_, err = tx.ExecContext(ctx, `INSERT INTO recipients (message_id, email, type)
            VALUES (?, ?, ?);`, message.ID, recipient.Email, recipient.Type)
		if err != nil {
			return fmt.Errorf("insert recipient: %w", err)
		}
	}

	for _, attachment := range attachments {
		_, err = tx.ExecContext(ctx, `INSERT INTO attachments
            (message_id, filename, content_type, data, size)
            VALUES (?, ?, ?, ?, ?);`,
			message.ID,
			attachment.Filename,
			attachment.ContentType,
			attachment.Data,
			attachment.Size,
		)
		if err != nil {
			return fmt.Errorf("insert attachment: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit message: %w", err)
	}
	return nil
}

func (s *Store) ListMessages(ctx context.Context, email, box, search, sort string, offset, limit int32) ([]MessageSummary, int32, error) {
	if limit <= 0 {
		limit = 10
	}
	if offset < 0 {
		offset = 0
	}

	baseQuery := " FROM messages m"
	whereQuery := ""
	args := []any{}

	switch box {
	case "sent":
		whereQuery = " WHERE m.from_email = ?"
		args = append(args, email)
	default:
		whereQuery = " WHERE EXISTS (SELECT 1 FROM recipients r WHERE r.message_id = m.id AND r.email = ?)"
		args = append(args, email)
	}

	search = strings.TrimSpace(search)
	if search != "" {
		whereQuery += " AND (m.subject LIKE ? OR m.from_email LIKE ? OR EXISTS (SELECT 1 FROM recipients r2 WHERE r2.message_id = m.id AND r2.email LIKE ?))"
		term := "%" + search + "%"
		args = append(args, term, term, term)
	}

	countQuery := "SELECT COUNT(1)" + baseQuery + whereQuery
	var totalCount int64
	if err := s.db.QueryRowContext(ctx, countQuery, args...).Scan(&totalCount); err != nil {
		return nil, 0, fmt.Errorf("count messages: %w", err)
	}
	if totalCount < 0 {
		totalCount = 0
	}
	if totalCount > int64(^uint32(0)>>1) {
		totalCount = int64(^uint32(0) >> 1)
	}

	orderBy := " ORDER BY m.created_at DESC, m.id DESC"
	switch sort {
	case "oldest", "asc":
		orderBy = " ORDER BY m.created_at ASC, m.id ASC"
	}

	listQuery := `SELECT m.id, m.from_email, m.subject, m.created_at,
		EXISTS(SELECT 1 FROM attachments a WHERE a.message_id = m.id) as has_attachments` + baseQuery + whereQuery + orderBy + " LIMIT ? OFFSET ?"
	listArgs := append([]any{}, args...)
	listArgs = append(listArgs, limit, offset)

	rows, err := s.db.QueryContext(ctx, listQuery, listArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("list messages: %w", err)
	}
	defer rows.Close()

	var messages []MessageSummary
	var ids []string
	for rows.Next() {
		var summary MessageSummary
		var createdAt int64
		if err := rows.Scan(
			&summary.ID,
			&summary.From,
			&summary.Subject,
			&createdAt,
			&summary.HasAttachments,
		); err != nil {
			return nil, 0, fmt.Errorf("scan message: %w", err)
		}
		summary.CreatedAt = time.Unix(createdAt, 0)
		messages = append(messages, summary)
		ids = append(ids, summary.ID)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("list messages: %w", err)
	}

	if len(ids) == 0 {
		return messages, int32(totalCount), nil
	}

	recipients, err := s.listRecipients(ctx, ids)
	if err != nil {
		return nil, 0, err
	}
	for i := range messages {
		messages[i].RecipientGroups = recipients[messages[i].ID]
	}
	return messages, int32(totalCount), nil
}

func (s *Store) GetMessage(ctx context.Context, email, id string) (Message, []Recipient, []Attachment, error) {
	var message Message
	var createdAt int64
	row := s.db.QueryRowContext(ctx, `SELECT id, from_email, subject, text_body, html_body, raw, raw_size, created_at
        FROM messages
        WHERE id = ? AND (from_email = ? OR EXISTS (SELECT 1 FROM recipients r WHERE r.message_id = messages.id AND r.email = ?));`,
		id, email, email)
	if err := row.Scan(
		&message.ID,
		&message.From,
		&message.Subject,
		&message.TextBody,
		&message.HTMLBody,
		&message.Raw,
		&message.RawSize,
		&createdAt,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Message{}, nil, nil, sql.ErrNoRows
		}
		return Message{}, nil, nil, fmt.Errorf("get message: %w", err)
	}
	message.CreatedAt = time.Unix(createdAt, 0)

	recipients, err := s.getRecipients(ctx, id)
	if err != nil {
		return Message{}, nil, nil, err
	}
	attachments, err := s.getAttachments(ctx, id)
	if err != nil {
		return Message{}, nil, nil, err
	}
	return message, recipients, attachments, nil
}

func (s *Store) DeleteMessage(ctx context.Context, email, id string) (bool, error) {
	result, err := s.db.ExecContext(ctx, `DELETE FROM messages
        WHERE id = ? AND (from_email = ? OR EXISTS (SELECT 1 FROM recipients r WHERE r.message_id = messages.id AND r.email = ?));`,
		id, email, email)
	if err != nil {
		return false, fmt.Errorf("delete message: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("delete message: %w", err)
	}
	return rows > 0, nil
}

func (s *Store) GetAttachment(ctx context.Context, email string, attachmentID int64) (Attachment, error) {
	var attachment Attachment
	row := s.db.QueryRowContext(ctx, `SELECT a.id, a.message_id, a.filename, a.content_type, a.data, a.size
        FROM attachments a
        JOIN messages m ON m.id = a.message_id
        WHERE a.id = ? AND (m.from_email = ? OR EXISTS (SELECT 1 FROM recipients r WHERE r.message_id = m.id AND r.email = ?));`,
		attachmentID, email, email)
	if err := row.Scan(
		&attachment.ID,
		&attachment.MessageID,
		&attachment.Filename,
		&attachment.ContentType,
		&attachment.Data,
		&attachment.Size,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Attachment{}, sql.ErrNoRows
		}
		return Attachment{}, fmt.Errorf("get attachment: %w", err)
	}
	return attachment, nil
}

func (s *Store) getRecipients(ctx context.Context, messageID string) ([]Recipient, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT email, type FROM recipients WHERE message_id = ? ORDER BY id;`, messageID)
	if err != nil {
		return nil, fmt.Errorf("get recipients: %w", err)
	}
	defer rows.Close()

	var recipients []Recipient
	for rows.Next() {
		var recipient Recipient
		if err := rows.Scan(&recipient.Email, &recipient.Type); err != nil {
			return nil, fmt.Errorf("get recipients: %w", err)
		}
		recipients = append(recipients, recipient)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("get recipients: %w", err)
	}
	return recipients, nil
}

func (s *Store) getAttachments(ctx context.Context, messageID string) ([]Attachment, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, message_id, filename, content_type, size FROM attachments WHERE message_id = ? ORDER BY id;`, messageID)
	if err != nil {
		return nil, fmt.Errorf("get attachments: %w", err)
	}
	defer rows.Close()

	var attachments []Attachment
	for rows.Next() {
		var attachment Attachment
		if err := rows.Scan(&attachment.ID, &attachment.MessageID, &attachment.Filename, &attachment.ContentType, &attachment.Size); err != nil {
			return nil, fmt.Errorf("get attachments: %w", err)
		}
		attachments = append(attachments, attachment)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("get attachments: %w", err)
	}
	return attachments, nil
}

func (s *Store) listRecipients(ctx context.Context, messageIDs []string) (map[string]map[string][]string, error) {
	if len(messageIDs) == 0 {
		return map[string]map[string][]string{}, nil
	}
	placeholders := strings.Repeat("?,", len(messageIDs))
	placeholders = strings.TrimSuffix(placeholders, ",")
	query := fmt.Sprintf(`SELECT message_id, email, type FROM recipients WHERE message_id IN (%s);`, placeholders)

	args := make([]any, len(messageIDs))
	for i, id := range messageIDs {
		args[i] = id
	}
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list recipients: %w", err)
	}
	defer rows.Close()

	result := make(map[string]map[string][]string)
	for rows.Next() {
		var messageID string
		var email string
		var rtype string
		if err := rows.Scan(&messageID, &email, &rtype); err != nil {
			return nil, fmt.Errorf("list recipients: %w", err)
		}
		if _, ok := result[messageID]; !ok {
			result[messageID] = map[string][]string{}
		}
		result[messageID][rtype] = append(result[messageID][rtype], email)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list recipients: %w", err)
	}
	return result, nil
}
