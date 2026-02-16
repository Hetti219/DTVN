# Contributing to DTVN

Thank you for your interest in contributing to DTVN! This document provides guidelines and information for contributors.

## Table of Contents

- [Code of Conduct](#code-of-conduct)
- [Getting Started](#getting-started)
- [Development Workflow](#development-workflow)
- [CI/CD Pipeline](#cicd-pipeline)
- [Running Tests](#running-tests)
- [Code Quality Standards](#code-quality-standards)
- [Pull Request Process](#pull-request-process)
- [Commit Message Guidelines](#commit-message-guidelines)

## Code of Conduct

- Be respectful and constructive
- Focus on what is best for the project and community
- Show empathy towards other community members

## Getting Started

### Prerequisites

- **Go 1.25.5 or later**
- **Make**
- **Git**
- **golangci-lint** (optional, for local linting)
- **gosec** (optional, for local security scanning)
- **govulncheck** (optional, for local vulnerability checking)

### Setting Up Your Development Environment

```bash
# Clone the repository
git clone https://github.com/Hetti219/DTVN.git
cd DTVN

# Set up development environment
make dev-setup

# Install optional development tools
go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
go install github.com/securego/gosec/v2/cmd/gosec@latest
go install golang.org/x/vuln/cmd/govulncheck@latest
```

## Development Workflow

1. **Fork the repository** on GitHub
2. **Clone your fork** locally
3. **Create a feature branch** from `main`
   ```bash
   git checkout -b feature/your-feature-name
   ```
4. **Make your changes** following our code quality standards
5. **Test your changes** thoroughly
6. **Commit your changes** with clear, descriptive messages
7. **Push to your fork** and **create a Pull Request**

## CI/CD Pipeline

Our CI/CD pipeline runs automatically on every push and pull request. It consists of several jobs:

### Required Checks (Must Pass)

1. **Lint** - Code linting with golangci-lint
2. **Format** - Code formatting check with gofmt
3. **Security** - Security scanning with gosec
4. **Vulnerability** - Vulnerability checking with govulncheck
5. **Build & Unit Tests** - Build all binaries and run unit tests
6. **Coverage** - Run tests with coverage reporting
7. **Build Matrix** - Cross-platform build verification (Ubuntu, macOS)

### Optional Checks

- **Integration Tests** - Run only on push to `main` or PRs with `run-integration` label

### Local CI Verification

Before pushing, run our local CI check script to catch issues early:

```bash
./scripts/ci-local-check.sh
```

This script runs the same checks as CI (where tools are available locally).

## Running Tests

### Unit Tests (Fast)

```bash
# Run all unit tests
make test-unit

# Run tests for a specific package
go test -v -short ./pkg/consensus/

# Run a specific test
go test -v -short -run TestSpecificFunction ./pkg/consensus/
```

### Integration Tests

```bash
# Run all integration tests
make test-integration

# Run with verbose output
make test-integration-verbose

# Run a specific integration test
make test-integration-single TEST=TestSingleTicketValidation

# Run in parallel (faster)
make test-integration-parallel

# Debug mode (keep artifacts)
make test-integration-debug
```

### Coverage

```bash
# Generate coverage report
make test-coverage

# View coverage in browser (opens coverage.html)
```

## Code Quality Standards

### Formatting

All code must be formatted with `gofmt`:

```bash
make fmt
```

### Linting

Code must pass `golangci-lint` checks:

```bash
make lint
```

Our linter configuration (`.golangci.yml`) checks for:
- Unchecked errors
- Code simplifications
- Security issues
- Code complexity
- Code duplications
- Common mistakes

### Security

- Never commit secrets, API keys, or credentials
- Use constant-time comparison for sensitive data
- Validate all external inputs
- Close resources properly (files, connections, etc.)

### Best Practices

- **Error handling**: Always check and handle errors appropriately
- **Comments**: Add comments for complex logic, but prefer self-documenting code
- **Function size**: Keep functions focused and under 50 lines when possible
- **Cyclomatic complexity**: Keep complexity under 15
- **Testing**: Write tests for new functionality
- **Documentation**: Update README.md and code comments as needed

### Code Review Checklist

Before submitting a PR, ensure:
- [ ] Code is formatted (`make fmt`)
- [ ] Code passes linting (`make lint` or will pass in CI)
- [ ] All tests pass (`make test-unit`)
- [ ] New code has tests
- [ ] go.mod is tidy (`make tidy`)
- [ ] No security issues
- [ ] Documentation is updated if needed

## Pull Request Process

1. **Ensure your PR**:
   - Has a clear, descriptive title
   - Describes what changes were made and why
   - References any related issues (#123)
   - Passes all CI checks

2. **PR Title Format**:
   ```
   <type>: <description>

   Examples:
   feat: add ticket expiration feature
   fix: resolve consensus deadlock on view change
   docs: update API documentation
   test: add integration tests for state sync
   refactor: simplify gossip engine
   perf: optimize BoltDB queries
   ```

3. **Types**:
   - `feat` - New feature
   - `fix` - Bug fix
   - `docs` - Documentation changes
   - `test` - Test additions or fixes
   - `refactor` - Code refactoring
   - `perf` - Performance improvements
   - `chore` - Maintenance tasks
   - `ci` - CI/CD changes

4. **Review Process**:
   - At least one maintainer approval required
   - All CI checks must pass
   - Address review feedback promptly
   - Keep PR scope focused and manageable

5. **Integration Tests**:
   - Add the `run-integration` label to run integration tests in CI
   - Integration tests automatically run for pushes to `main`

## Commit Message Guidelines

### Format

```
<type>: <subject>

<body>

<footer>
```

### Example

```
feat: implement ticket expiration with configurable TTL

Add configurable time-to-live for tickets in the ISSUED state.
Expired tickets are automatically cleaned up by a background
worker that runs every 5 minutes.

Closes #42
```

### Rules

- **Subject line**:
  - Use imperative mood ("add" not "added" or "adds")
  - No period at the end
  - Under 72 characters

- **Body**:
  - Explain what and why, not how
  - Wrap at 72 characters
  - Separate from subject with a blank line

- **Footer**:
  - Reference issues and PRs
  - Note breaking changes with `BREAKING CHANGE:`

## Project-Specific Guidelines

### Modifying Protocol Buffers

If you modify `.proto` files:

```bash
# Regenerate Go code
make proto

# Commit both .proto and .pb.go files
git add proto/messages.proto proto/messages.pb.go
```

### Adding Dependencies

```bash
# Add dependency
go get github.com/example/package

# Tidy modules
make tidy

# Verify everything still works
make build
make test-unit
```

### PBFT Consensus Changes

**CRITICAL**: Changes to PBFT must maintain Byzantine fault tolerance:
- Quorum size must remain 2f+1
- Message signing/verification must not be bypassed
- View change protocol ensures liveness
- Consult CLAUDE.md before modifying consensus

### Adding API Endpoints

1. Add handler to `pkg/api/server.go`
2. Register route in `setupRoutes()`
3. Add proxy handler in `pkg/supervisor/proxy.go` (if needed)
4. Update `web/static/js/api.js`
5. Update API documentation in README.md
6. Add tests

## Getting Help

- **Documentation**: Check README.md and CLAUDE.md
- **Issues**: Search existing issues or create a new one
- **Discussions**: Use GitHub Discussions for questions

## Recognition

Contributors will be recognized in:
- Git commit history
- Release notes
- Project README (for significant contributions)

Thank you for contributing to DTVN! 🎉
