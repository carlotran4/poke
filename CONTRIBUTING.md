# Contributing

Poke is a local messaging building block. Keep agent orchestration, scheduling,
UI, remote services and model execution outside the core. Discuss scope expansion
before implementation. Preserve mail history and distinguish submission from receipt.

Use Go 1.26.11+ and a C compiler. Before opening a pull request:

```sh
go fmt ./...
go test ./...
go test -race ./...
go vet ./...
go run ./examples/embedded
node --test adapters/pi.test.mjs
```

Changes to delivery require tests for competing connections, crash uncertainty,
stale identity and monotonic receipts. Transport changes need deterministic native
frames/command assertions, plus opt-in installed-harness evidence when available.
Never use personal sessions or databases as fixtures. Do not place credentials,
transcripts, database files or native endpoint files in issues or commits.

Keep README examples executable, update CHANGELOG.md and the closest contract doc,
and describe verification and unverified native behavior in the pull request.
Keep dependency upgrades focused. A host must retain its existing lifecycle locks
and transaction boundaries when adopting Poke; do not weaken those to simplify API use.
