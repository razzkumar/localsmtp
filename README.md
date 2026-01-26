# LocalSMTP

A lightweight SMTP capture server for local development and testing. Catch all outgoing emails from your application without sending them to real recipients.

## Features

- **SMTP Server** - Captures all emails on port 2025
- **Web Interface** - Modern React UI to browse captured emails
- **Email-only Login** - No passwords, just enter any email address
- **Per-user Views** - Inbox and sent views based on your login email
- **Real-time Updates** - Live email notifications via Server-Sent Events
- **Attachments** - Full support for email attachments
- **Search** - Find emails by subject, sender, or recipient
- **SQLite Storage** - Lightweight persistence (or in-memory mode)
- **Single Binary** - No external dependencies, easy deployment
- **Docker Ready** - Multi-arch images for amd64 and arm64

## Installation

### Docker (Recommended)

```bash
docker run -d \
  --name localsmtp \
  -p 3025:3025 \
  -p 2025:2025 \
  -v localsmtp-data:/data \
  -e DB_PATH=/data/localsmtp.db \
  -e AUTH_SECRET=your-secret-here \
  razzkumar/localsmtp:latest
```

### Binary Release

Download the latest release from the [Releases](https://github.com/razzkumar/localsmtp/releases) page.

```bash
# Linux/macOS
chmod +x localsmtp
./localsmtp

# Windows
localsmtp.exe
```

### From Source

Requires Go 1.25+ and Node.js 18+.

```bash
# Clone the repository
git clone https://github.com/razzkumar/localsmtp.git
cd localsmtp

# Build the web UI
cd web
npm install
npm run build
cd ..

# Run the server
go run ./cmd/localsmtp
```

## Usage

1. Start LocalSMTP using one of the methods above
2. Open http://localhost:3025 in your browser
3. Log in with any email address (e.g., `dev@example.com`)
4. Configure your application to use `localhost:2025` as the SMTP server
5. Send test emails - they'll appear in your LocalSMTP inbox

### Configuration

LocalSMTP is configured via environment variables. Create a `.env` file or set them directly.

| Variable | Default | Description |
|----------|---------|-------------|
| `HTTP_PORT` | `3025` | Web interface port |
| `SMTP_PORT` | `2025` | SMTP server port |
| `DB_PATH` | _(empty)_ | SQLite database path. Empty = in-memory (no persistence) |
| `AUTH_SECRET` | _(empty)_ | Secret for signing session cookies. Set this in production |

### Example: Send Test Email

```go
package main

import (
    "net/smtp"
)

func main() {
    from := "sender@example.com"
    to := "recipient@example.com"
    subject := "Test Email"
    body := "Hello from LocalSMTP!"

    msg := []byte("From: " + from + "\r\n" +
        "To: " + to + "\r\n" +
        "Subject: " + subject + "\r\n\r\n" +
        body + "\r\n")

    smtp.SendMail("localhost:2025", nil, from, []string{to}, msg)
}
```

### Docker Compose

```yaml
version: "3.8"
services:
  localsmtp:
    image: razzkumar/localsmtp:latest
    ports:
      - "3025:3025"
      - "2025:2025"
    volumes:
      - localsmtp-data:/data
    environment:
      - DB_PATH=/data/localsmtp.db
      - AUTH_SECRET=change-me-in-production

volumes:
  localsmtp-data:
```

## Development

### Prerequisites

- Go 1.25+
- Node.js 18+
- npm

### Running in Development

```bash
# Terminal 1: Start the backend with hot reload (requires air)
air

# Terminal 2: Start the frontend dev server
cd web
npm run dev
```

### Building

```bash
# Build web assets
cd web && npm run build && cd ..

# Build the binary
go build -o localsmtp ./cmd/localsmtp
```

## Contributing

Contributions are welcome! Please see [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines.

## License

MIT License - see [LICENSE](LICENSE) for details.
