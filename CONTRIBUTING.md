# Contributing Guide

Thank you for your interest in the CodeAgent project! We welcome all forms of contributions.

## How to Contribute

### Reporting Bugs

If you find a bug, please:

1. Search existing GitHub Issues to see if it has already been reported
2. If not, create a new Issue with:
   - Detailed bug description
   - Steps to reproduce
   - Expected vs actual behavior
   - Environment information (OS, Go version, etc.)
   - Relevant logs

### Feature Requests

If you have a feature suggestion:

1. Search existing GitHub Issues to see if it has been discussed
2. Create a new Issue with detailed description of:
   - Feature requirements
   - Use cases
   - Expected outcomes

### Code Contributions

#### Development Environment Setup

1. Fork the project to your GitHub account
2. Clone your fork:

   ```bash
   git clone https://github.com/your-username/codeagent.git
   cd codeagent
   ```

3. Add upstream repository:

   ```bash
   git remote add upstream https://github.com/qiniu/codeagent.git
   ```

4. Create a feature branch:
   ```bash
   git checkout -b feature/your-feature-name
   ```

#### Development Process

1. **Code Standards**

   - Follow Go official coding standards
   - Use `gofmt` to format code
   - Run `go vet` to check for code issues

2. **Testing**

   - Write tests for new features
   - Ensure all tests pass:
     ```bash
     make test
     ```

3. **Commit Standards**

   - Use clear commit messages
   - Format: `type(scope): description`
   - Examples:
     - `feat(webhook): add signature validation`
     - `fix(agent): resolve race condition in workspace cleanup`
     - `docs(readme): update installation instructions`

4. **Pull Request**
   - Ensure code passes all checks
   - Provide detailed PR description
   - Include test cases and documentation updates

#### Code Review

All code changes require code review:

1. Ensure CI checks pass
2. At least one maintainer approval required
3. Address all review comments

## Development Guide

### Project Structure

```
codeagent/
├── cmd/                    # Command line tools
├── internal/              # Internal packages
│   ├── agent/            # Core agent logic
│   ├── code/             # AI code generation
│   ├── config/           # Configuration management
│   ├── github/           # GitHub API client
│   ├── webhook/          # Webhook handling
│   └── workspace/        # Workspace management
├── pkg/                  # Public packages
└── docs/                 # Documentation
```

### Testing Guide

- Unit tests: `go test ./...`
- Integration tests: Use `test-local-mode.sh` script
- Coverage: `go test -coverprofile=coverage.out ./...`

### Development Environment Configuration

1. Set required environment variables:

   ```bash
   export GITHUB_TOKEN="your-token"
   export CLAUDE_API_KEY="your-key"  # or GEMINI_API_KEY
   export WEBHOOK_SECRET="your-secret"
   ```

2. Start in development mode:
   ```bash
   ./scripts/start.sh -p claude  # Local CLI mode
   ```

## Code of Conduct

- Respect all contributors
- Maintain professional and friendly communication
- Welcome new contributors
- Provide constructive feedback

## Contact

For questions, please contact us through:

- GitHub Issues: Report bugs and feature requests
- GitHub Discussions: General discussions and questions

Thank you for your contributions!
