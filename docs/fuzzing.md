# Release-candidate fuzzing

The public repository uses Go's native, coverage-guided fuzzer through the
shared `venus-os-ci-toolkit` release pipeline. The opt-in manifest is
[`.github/fuzz.json`](../.github/fuzz.json); the shared runner owns toolchain
setup, time limits, isolation and evidence collection. This is project-operated
GitHub Actions testing, not enrollment in Google's hosted OSS-Fuzz service.

## When it runs

Mutation fuzzing runs only for an explicit **release candidate** request from
the default branch of this public repository. Pull requests, branch pushes,
nightlies and betas do not start mutation campaigns. Private repositories are
excluded even if they accidentally copy the configuration. Each target gets
60 seconds, with two workers; the three campaigns total at most 180 seconds of
mutation time, plus bounded setup/compilation and evidence handling.

The RC must pass fuzzing before publication. A finding fails the candidate: fix
the defect, preserve a small regression input, review and merge the fix, then
request a new RC. Stable promotion reuses the accepted candidate's source and
artifact evidence; it does not run another campaign or rebuild the artifacts.
The README's **Fuzzing: RC only** badge documents this policy; it is not a green
test result. Consult the exact RC Actions run and its evidence for actual results.

Normal `go test` runs still execute the small checked-in seed corpus and any
regression cases. That fast deterministic testing does not enable fuzz mutation
on every PR or beta.

## What the targets exercise

- `FuzzNormalizeEndpoint` calls the production gateway/Home Assistant endpoint
  normalizer with arbitrary URL text. Accepted values must remain absolute
  HTTP(S) destinations, have valid ports, contain no credentials/query/fragment,
  and stay unchanged when normalized again. Rejected values cannot return a
  usable destination.
- `FuzzSameOriginPolicy` calls the production browser-origin guard with arbitrary
  Origin, Host and Fetch Metadata values. Forwarded headers cannot change the
  authority decision, duplicate Origin values must fail closed, and omission by
  native clients does not bypass the Fetch Metadata policy.
- `FuzzControllerCommand` exercises the production controller-state decoder,
  override/tariff validators and wire encoder. An accepted override must retain
  an exact signed 32-bit integer (or the explicit stop value `null`); accepted
  tariffs cannot exceed their 100,000-byte wire budget. Encoding and decoding
  cannot change whether the command is valid.

These targets never open sockets, resolve hosts, connect to MQTT/Home Assistant,
or issue device commands. Dependencies are fetched during setup; mutation runs
have no network access in CI. Inputs are bounded (4 KiB endpoints, 8 KiB combined
origin fields, 128 KiB controller JSON) to focus the budget on protocol behavior.
They do not claim to test physical-device acceptance, every dashboard handler,
or every third-party dependency.

## Reproduce and extend

Use the Go version declared in `go.mod`, download dependencies, then run:

```sh
go mod download
go test ./internal/httpendpoint ./internal/auth ./internal/state
go test ./internal/httpendpoint -run='^$' -fuzz='^FuzzNormalizeEndpoint$' -fuzztime=60s -parallel=2
go test ./internal/auth -run='^$' -fuzz='^FuzzSameOriginPolicy$' -fuzztime=60s -parallel=2
go test ./internal/state -run='^$' -fuzz='^FuzzControllerCommand$' -fuzztime=60s -parallel=2
```

Go prints the minimized failing input path and a `-run` command. Preserve that
input under the target package's `testdata/fuzz/<FuzzName>/` and run the printed
command against the fix, followed by the ordinary package tests and a fresh
campaign. Never add production tokens, personal telemetry or device identifiers
to a seed or an uploaded artifact. Report security-sensitive findings privately
using [SECURITY.md](../SECURITY.md) before publishing a reproducer.

New targets should call actual application boundaries and assert independent
invariants, rather than only checking the standard JSON parser. Keep them
deterministic, bounded and free of external I/O. Add their command and budget to
the manifest; the shared runner rejects excessive aggregate budgets. See
[Go's fuzzing documentation](https://go.dev/doc/security/fuzz/) for corpus format,
minimization and local troubleshooting.
