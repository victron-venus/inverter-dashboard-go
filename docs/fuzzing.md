# Release-candidate fuzzing

See the [illustrated RC fuzzing process and verified rollout results](https://github.com/victron-venus/venus-os-ci-toolkit/blob/main/docs/rc-fuzzing-results.md)
for the shared release flow, evidence requirements and a dated cross-project
snapshot. The project-specific scope and reproduction guidance below still apply.

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

## Release workflow retries

Fresh release inputs are selected by the exact GitHub workflow run ID and run
attempt.

Before publication starts, use **Re-run all jobs** only if the allocator and
release policy permit reuse of that run's reservation. Recreate every required
input in the same attempt; failed-only or individual retries can leave inputs
missing, and older artifacts must not fill those gaps.

For a fresh candidate or final build, once publication has started
(`begin_publication`), or its external publication result is partial or uncertain,
inspect the existing release, tags and assets before retrying. A consumed build
number requires a new workflow run and reservation authorized by the release
policy; an existing immutable tag may also require a new base version. Never
reset the release ledger, overwrite assets or retag to force a retry. Historical
byte promotion of an accepted RC keeps its reserved number and follows the
existing promotion policy.

Artifacts from older attempts may remain available for diagnostics; they are not
inputs to the new attempt and do not need to be deleted. Filtering artifact names
alone does not prove package completeness or receipt identity. Complete package
inventory and receipt identity checks remain separate release requirements.

See the shared [package identity and retry contract](https://github.com/victron-venus/venus-os-ci-toolkit/blob/main/docs/VERSIONING.md#package-identity-across-reruns).

### Package receipt identity

Fresh publication verifies each staged package receipt against the independently
supplied repository, source commit, workflow run and attempt. Receipts created
before this migration, or offline without an Actions identity, cannot qualify a
fresh publication. Run all required producers in one complete workflow attempt
after migration; an earlier attempt cannot fill missing inputs.

Historical promotion and retained-evidence validation keep their existing checks.
Receipt coverage validates the staged files; the expected packages and platforms
still require a separate completeness check. Existing release approval and device
acceptance policies continue to apply.

See the shared [receipt migration guide](https://github.com/victron-venus/venus-os-ci-toolkit/blob/main/docs/release-workflow.md#migrate-package-receipt-identity-without-replacing-the-release-engine).

### Complete package producer inventory

`versioning.required_build_receipts` declares the exact package and signature
receipt filenames required by this project's final publisher. Each authenticated
receipt must match the repository, source commit, workflow run and attempt. A
missing producer or an unexpected receipt stops fresh publication, even when the
remaining packages are valid. RC fuzz evidence is checked separately; pre-signing
checks do not require a signature receipt that has not been created yet.

Before publication begins, rerun all required producers only when the allocator
and release policy permit reusing that reservation. After publication begins or
its outcome is uncertain, inspect existing tags, releases and assets before a
policy-authorized new run; never reset the ledger or mix earlier-attempt packages.
Historical RC promotion keeps its accepted source policy. Keep this explicit
inventory aligned with producer changes; it does not replace package-format,
signature or device acceptance checks.
