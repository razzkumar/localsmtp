# LocalSMTP

LocalSMTP is a modern SMTP capture tool for easy test. It provides a Go 1.25 backend with a React + Vite UI, email-only login, and per-user inbox/sent views backed by SQLite.

## Features

- SMTP capture server on port 2025
- Email-only login (no password, any address)
- Per-user inbox + sent views
- Real-time updates via Server-Sent Events
- Local-only send form for test emails
- SQLite storage for messages and attachments

## Quick start

1. Install Go 1.25 and Node 18+.
2. Build the UI assets:

```bash
cd web
npm install
npm run build
```

3. Start the backend:

```bash
go run ./cmd/localsmtp
```

4. Open `http://localhost:3025` and log in with any email.
5. Configure your app to use SMTP on `localhost:2025`.
6. Optional: set `DB_PATH=localsmtp.db` to persist messages.

## Build the UI for production

```bash
cd web
npm install
npm run build
```

Then run the Go server. It embeds `web/dist` into the binary.

## Configuration

| Variable | Default | Description |
| --- | --- | --- |
| `HTTP_PORT` | `3025` | HTTP server port |
| `SMTP_PORT` | `2025` | SMTP server port |
| `DB_PATH` | empty (in-memory) | SQLite database path (set for persistence) |
| `AUTH_SECRET` | empty | Session signing secret (recommended in production) |
