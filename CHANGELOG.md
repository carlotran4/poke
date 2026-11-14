# Changelog

## 0.1.0

Initial experimental extraction from Kanbi's cross-harness mailbox.

- Durable SQLite mail, idempotent sends, parent-derived replies and explicit ack.
- Atomic notification claims, restart uncertainty fencing and explicit retries.
- Shared Claude socket, Codex queue and Pi extension adapters and launch assets.
- Host-owned transactions and lifecycle guards for embedded metaharnesses.
- Standalone JSON CLI with named immutable attempts, manual inboxes and native launch.
- Linux/macOS CI, race tests, native receiver fixtures and a standalone Go example.

Kanbi retains its database migrations, Operator transactions, runtime ownership,
verified ref capture and best-effort Copilot terminal adapter. Standalone Poke does
not discover Codex refs, provide remote transport, or guarantee exactly-once turns.
