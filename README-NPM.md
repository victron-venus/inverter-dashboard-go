# npm and the Go dashboard

The application is built and run with Go. It is not an npm-distributed server, and Node.js is not required for an installed binary or container.

The historical [package.json](package.json) contains convenience wrappers around Go, Make and Docker commands. Its `1.0.0` package version is not the application's release version. Some legacy scripts assume a nonexistent `config/` sample layout or use `go run main.go`, which does not include the other files in the root Go package. `npm install` also invokes `go mod tidy`, potentially changing dependency files. Prefer the supported commands below rather than these wrappers.

## Build and run

Use the toolchain declared in [go.mod](go.mod), from the repository root:

```sh
go mod download
go build -trimpath -ldflags "-X main.Version=$(cat VERSION)" -o inverter-dashboard .
./inverter-dashboard --version
cp config.yaml.example config.yaml
chmod 600 config.yaml
```

Edit the configuration, provide private authentication settings, then run `./inverter-dashboard` from that directory. Do not start an unconfigured process against a real installation. For a development run that includes the complete package, use `go run .` after the same configuration review.

[README.md](README.md) explains installation; [the configuration reference](docs/configuration.md) explains working-directory lookup and precedence. Settings and Web Push storage require persistent writable paths independent of the executable.

## Test and contribute

The authoritative full local check entry point is:

```sh
bash scripts/ci.sh
```

Install its required Go, golangci-lint and govulncheck tools first. For a focused change, run the relevant package tests and the configured pre-commit hooks, then follow [CONTRIBUTING.md](CONTRIBUTING.md) and [the developer guide](docs/development.md). Do not infer release readiness from `npm test` alone.

## Frontend work

Vue source development belongs in [inverter-dashboard-vue](https://github.com/victron-venus/inverter-dashboard-vue), using that repository's Node version, lockfile and checks. Do not edit generated files in `internal/html/vue-ui` or install frontend dependencies here to rebuild them. The Go repository embeds a reviewed official frontend artifact and records its provenance in [vue-ui-source.json](internal/html/vue-ui-source.json).

Release versions come from [VERSION](VERSION) and the saved release plan, not npm's `version` command. [RELEASING.md](RELEASING.md) defines publication and promotion.
