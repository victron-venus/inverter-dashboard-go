# NPM Commands Guide

This project now supports npm commands for familiar Node.js-style workflows alongside the existing Makefile.

## Quick Start with NPM

```bash
# Install dependencies (uses go mod)
npm install

# Create config file from sample
npm run setup

# Build the project
npm run build

# Run tests
npm test

# Start the application
npm start
```

## Available NPM Scripts

### Core Commands
- **`npm install`** - Install Go dependencies (go mod download && go mod tidy)
- **`npm run build`** - Build the Go binary (go build -o inverter-dashboard .)
- **`npm run build:release`** - Build for all platforms (make release)
- **`npm start`** - Run the compiled binary
- **`npm run run`** - Run directly with go run (development)

### Development Commands
- **`npm run dev`** - Run with live reload using Air tool (requires `air` to be installed)
- **`npm test`** - Run all Go tests (go test -v ./...)
- **`npm run test:watch`** - Run tests (alias for npm test)
- **`npm run test:coverage`** - Run tests with coverage report
- **`npm run lint`** - Run golangci-lint linter
- **`npm run lint:fix`** - Run linter with auto-fix
- **`npm run fmt`** - Format Go code (go fmt ./...)
- **`npm run fmt:check`** - Check if code is formatted

### Utility Commands
- **`npm run clean`** - Remove build artifacts and coverage files
- **`npm run setup`** - Create config.yaml from sample if it doesn't exist
- **`npm run version`** - Show current version from VERSION file
- **`npm run help`** - Display all available npm scripts

### Docker Commands
- **`npm run docker:build`** - Build Docker image
- **`npm run docker:run`** - Run Docker container
- **`npm run docker:stop`** - Stop and remove Docker container
- **`npm run docker:clean`** - Stop container and remove image

## NPM vs Makefile Comparison

| Task | NPM Command | Makefile Equivalent |
|------|------------|-------------------|
| Install deps | `npm install` | `make deps` |
| Build | `npm run build` | `make build` |
| Build all platforms | `npm run build:release` | `make release` |
| Run tests | `npm test` | `make test` |
| Run app | `npm start` | `make run` |
| Clean | `npm run clean` | `make clean` |
| Lint | `npm run lint` | `make lint` |
| Format | `npm run fmt` | `make fmt` |

## Initial Setup with NPM

```bash
# Clone repository
git clone https://github.com/victron-venus/inverter-dashboard-go.git
cd inverter-dashboard-go

# Install dependencies
npm install

# Setup configuration
npm run setup

# Edit config/config.yaml with your settings

# Build and run
npm run build
npm start
```

## Development Workflow

```bash
# 1. Install dependencies
npm install

# 2. Setup config
npm run setup
# Edit config/config.yaml

# 3. Run tests
npm test

# 4. Build
npm run build

# 5. Run the app
npm start

# Or use live reload during development
npm run dev  # Requires Air to be installed
```

## Testing with NPM

```bash
# Run all tests
npm test

# Run tests with coverage report
npm run test:coverage
# Opens coverage.html in browser
```

## CI/CD Integration with NPM

For CI/CD pipelines that expect Node.js projects:

```yaml
# Example GitHub Actions workflow
name: Build and Test
on: [push, pull_request]
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3
      - uses: actions/setup-go@v4
        with:
          go-version: '1.22'
      - uses: actions/setup-node@v3
        with:
          node-version: '18'
      - run: npm install
      - run: npm test
      - run: npm run build
```

## Requirements

- Node.js 14.0.0 or higher
- Go 1.22 or higher
- npm (usually comes with Node.js)

## NPM-Specific Files

- **`package.json`** - NPM configuration with all scripts
- **`package-lock.json`** - Auto-generated lock file
- **`.npmignore`** - Files to ignore when publishing to npm registry
- **`scripts/dev.sh`** - Helper script for dev command

## Why NPM for a Go Project?

Using NPM commands provides:

1. **Familiarity** for JavaScript/Node.js developers
2. **Consistency** with frontend projects in monorepos
3. **Tooling integration** with IDEs that expect npm scripts
4. **CI/CD compatibility** with existing Node.js-based pipelines
5. **Unified interface** - same commands work regardless of backend language

## Troubleshooting

### npm: command not found
```bash
# Install Node.js and npm
curl -fsSL https://deb.nodesource.com/setup_18.x | sudo -E bash -
sudo apt-get install -y nodejs
```

### Go command fails in npm scripts
Make sure Go is installed and in your PATH:
```bash
go version
```

### Permission denied for scripts/dev.sh
```bash
chmod +x scripts/dev.sh
```

## Migration from Makefile to NPM

Existing Make commands continue to work. You can gradually migrate to NPM commands:

```bash
# Instead of:
make build

# Use:
npm run build

# Both work! Use whichever you prefer.
```
