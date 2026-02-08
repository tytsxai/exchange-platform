# Contributing to OpenExchange

Thank you for your interest in contributing to OpenExchange! This document provides guidelines and instructions for contributing.

## 📋 Table of Contents

- [Code of Conduct](#code-of-conduct)
- [Getting Started](#getting-started)
- [Development Process](#development-process)
- [Submitting Changes](#submitting-changes)
- [Style Guidelines](#style-guidelines)
- [Testing](#testing)
- [Documentation](#documentation)

## Code of Conduct

This project follows the [Contributor Covenant Code of Conduct](https://www.contributor-covenant.org/). By participating, you are expected to uphold this code.

## Getting Started

### Prerequisites

- Go 1.25+
- Docker & Docker Compose
- PostgreSQL 15+
- Redis 7+
- Git

### Setting Up Development Environment

1. Fork the repository on GitHub
2. Clone your fork locally:

```bash
git clone https://github.com/YOUR-USERNAME/exchange-platform.git
cd exchange-platform
```

3. Add the original repository as upstream:

```bash
git remote add upstream https://github.com/original-owner/exchange-platform.git
```

4. Create a feature branch:

```bash
git checkout -b feature/your-feature-name
```

## Development Process

### 1. Keep Your Fork Updated

```bash
git fetch upstream
git merge upstream/master
```

### 2. Make Changes

- Follow the existing code style
- Write clear, commented code
- Keep changes focused and atomic

### 3. Test Your Changes

```bash
# Run tests for the changed module
go test ./exchange-common/...

# Run all tests
go test ./...

# Check for race conditions
go test -race ./...
```

### 4. Linting and Formatting

```bash
# Format code
gofmt -w .

# Run linter (install golangci-lint first)
golangci-lint run
```

## Submitting Changes

### Pull Request Process

1. **Update your branch** with upstream changes
2. **Ensure tests pass**
3. **Update documentation** if needed
4. **Write a clear PR description**:
   - What changes were made
   - Why these changes are needed
   - Any breaking changes
   - Screenshots for UI changes

5. **Link related issues**

### PR Title Convention

Use conventional commits format:

```
feat: Add new order type support
fix: Resolve race condition in matching engine
docs: Update API documentation
refactor: Simplify order validation logic
```

## Style Guidelines

### Go Code

- Follow [Effective Go](https://golang.org/doc/effective_go)
- Use `gofmt` for formatting
- Add comments for exported functions and types
- Keep functions small and focused
- Use meaningful variable names

### Error Handling

- Return errors instead of panicking (except in main)
- Use wrapped errors with context: `fmt.Errorf("failed to connect: %w", err)`
- Don't ignore errors with `_`

### Comments

```go
// Good: Clear, concise comments
// ProcessOrder validates and processes a new order request.

// Bad: Redundant comments
// i is an integer. i++ increments i.
i++
```

### Naming

- Use CamelCase for exported identifiers
- Use short names for short scopes (`i` for loop counter)
- Avoid stutter (`order.OrderService` → `order.Service`)

## Testing

### Unit Tests

- Write tests for all exported functions
- Use table-driven tests when appropriate
- Test edge cases and error conditions

```go
func TestProcessOrder(t *testing.T) {
    tests := []struct {
        name    string
        order   *Order
        want    error
    }{
        {"valid order", validOrder, nil},
        {"invalid price", invalidPriceOrder, ErrInvalidPrice},
    }
    // ...
}
```

### Integration Tests

- Test service interactions
- Use Docker Compose for test infrastructure

## Documentation

### Code Documentation

- Document all exported APIs
- Include example usage where helpful
- Update doc comments when changing behavior

### README Updates

- Update README for new features
- Add examples for new endpoints
- Update architecture diagrams if changed

## ⚠️ Important Notes

### Financial Software

This is financial software. Changes affecting:

- **Order processing** → Must be thoroughly tested
- **Matching logic** → Requires mathematical validation
- **Clearing/settlement** → Needs audit trail verification
- **Security** → Must pass security review

### Security-Sensitive Changes

For changes affecting security:

1. Describe the security implications
2. Document the threat model
3. Get review from maintainers
4. Consider independent security audit

## Questions?

- Open an issue for bugs
- Use discussions for questions
- Reach out to maintainers

## Recognition

Contributors will be listed in the README and release notes.

---

Thank you for contributing to OpenExchange! 🙏
