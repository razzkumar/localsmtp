package store

import "time"

type User struct {
	Email     string
	CreatedAt time.Time
	LastLogin time.Time
}

type Message struct {
	ID        string
	From      string
	Subject   string
	TextBody  string
	HTMLBody  string
	Raw       []byte
	RawSize   int64
	CreatedAt time.Time
}

type Recipient struct {
	Email string
	Type  string
}

type Attachment struct {
	ID          int64
	MessageID   string
	Filename    string
	ContentType string
	Data        []byte
	Size        int64
}

type MessageSummary struct {
	ID              string
	From            string
	Subject         string
	CreatedAt       time.Time
	HasAttachments  bool
	RecipientGroups map[string][]string
}
