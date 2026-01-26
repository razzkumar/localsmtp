# Contributing to LocalSMTP

Thank you for your interest in contributing to LocalSMTP! This document provides guidelines and information for contributors.

## Getting Started

1. Fork the repository
2. Clone your fork locally
3. Set up the development environment (see below)
4. Create a feature branch from `main`
5. Make your changes
6. Submit a pull request

## Development Environment

### Prerequisites

- Go 1.25 or later
- Node.js 18 or later
- npm

### Setup

```bash
# Clone your fork
git clone https://github.com/razzkumar/localsmtp.git
cd localsmtp

# Install frontend dependencies
cd web
npm install
cd ..

# Run in development mode
# Terminal 1: Backend with hot reload
air

# Terminal 2: Frontend dev server
cd web
npm run dev
```

### Project Structure

```
localsmtp/
├── cmd/localsmtp/     # Application entry point
├── internal/          # Internal packages
│   ├── config/        # Configuration loading
│   ├── server/        # HTTP server and handlers
│   ├── smtp/          # SMTP server
│   └── store/         # SQLite storage layer
├── web/               # React frontend
│   ├── src/           # Source files
│   └── dist/          # Built assets (generated)
├── example/           # Example code
└── .github/           # GitHub workflows
```

## Making Changes

### Code Style

- **Go**: Follow standard Go conventions. Run `go fmt` before committing
- **TypeScript/React**: Follow the existing code style in the `web/` directory

### Commit Messages

Write clear, concise commit messages:

- Use the present tense ("Add feature" not "Added feature")
- Use the imperative mood ("Fix bug" not "Fixes bug")
- Keep the first line under 72 characters
- Reference issues when applicable (e.g., "Fix #123")

Examples:
```
feat: add email search functionality
fix: resolve attachment download issue
docs: update installation instructions
refactor: simplify session handling
```

### Testing

Before submitting a PR:

1. Ensure the application builds without errors:
   ```bash
   cd web && npm run build && cd ..
   go build ./cmd/localsmtp
   ```

2. Test your changes manually by running the application

3. Verify Docker builds work:
   ```bash
   docker build -t localsmtp:test .
   ```

## Pull Requests

1. Update documentation if your changes affect user-facing functionality
2. Keep PRs focused on a single feature or fix
3. Fill out the PR template with relevant details
4. Be responsive to feedback and review comments

## Reporting Issues

When reporting bugs, please include:

- LocalSMTP version or commit hash
- Operating system and version
- Steps to reproduce the issue
- Expected vs actual behavior
- Relevant logs or error messages

## Feature Requests

Feature requests are welcome! Please:

- Check existing issues to avoid duplicates
- Clearly describe the use case
- Explain why existing features don't meet your needs

## Questions

If you have questions, feel free to:

- Open a discussion on GitHub
- Check existing issues for similar questions

## License

By contributing to LocalSMTP, you agree that your contributions will be licensed under the MIT License.
