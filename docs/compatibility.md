# Compatibility and verification

Poke v0.1 targets Linux and macOS with Go 1.26.11+ and CGO SQLite.
It intentionally has no Windows runtime support in this release: the local host
uses Unix file locks and Claude uses Unix sockets.

Native adapters are capability-dependent. Pi needs extensions, persistent sessions
and native follow-up APIs. Claude needs SessionStart hooks and its inbox socket.
Codex needs `queue --thread` and a verified native ref supplied by the host.
Poke's standalone CLI does not implement native session-ref discovery. Canonical
native command setup is supported; hidden wrapper behavior is not guessed.

| Evidence | What it proves |
| --- | --- |
| Poke unit/race tests | Durable transitions, cross-connection claim/send fencing, reply checks, local lifecycle rules, private assets, socket frames and recipient environment |
| Poke Pi receiver tests | Idle/editor guards, post-I/O recheck, native entry reconciliation and host protocol selection using a deterministic extension fixture |
| Kanbi focused/full suites | Embedded compatibility with historical rows, Operators, runtime policy and terminal fallback |
| `POKE_NATIVE_PI_TESTS=1 python3 scripts/pi-native-smoke.py` | Actual installed engine can load the shared extension and take a native turn without account credentials |
| Authenticated host tests | Actual installed harness/model exchange under that host's lifecycle; opt-in and account-dependent |

Poke's source starts from Kanbi's cross-harness mailbox. Prior Kanbi verification
included authenticated Pi/Codex exchange, Claude against a local provider fixture,
and deterministic Copilot terminal submission. Those historical results do not
establish compatibility with every future installed harness version.

Run the deterministic checks in CONTRIBUTING.md before native experiments. Record
exact installed versions, account restrictions and whether the model actually
fetched/acknowledged mail. Successful socket writes alone are not model reception.

Protocol and standalone schema versions are reported by `poke version`. The
standalone host refuses a newer schema and refuses to adopt a pre-existing host
mailbox. Go APIs are experimental until v1. Back up SQLite consistently using its
backup API or after closing all connections; copy endpoint credentials only as
machine-local private state, never as a portable mail archive.

If a standalone launcher is killed without cleanup, explicitly `poke retire NAME`
before treating that attempt as closed. Poke is not a process supervisor. Mail
history survives retirement; launching a replacement requires a new attempt name.

## Extraction verification (2026-11-14)

The initial extraction passed Go tests, race checks, vet, the independent Go
example, six Node Pi receiver cases, and the standalone installed-Pi offline smoke
with Pi 0.84.2. Kanbi also passed full/focused tests, vet, isolated Herdr smoke,
the installed-Pi offline fixture, and authenticated Pi 0.84.2 / Codex 0.169.0
launch/focus/exit/resume tests. Claude Code 2.1.319's offline host fixture was blocked
by Herdr's `agent_not_ready` during startup before receiver registration; native
Claude reception was not re-established in that environment. Socket framing and
receipt behavior passed deterministic tests. No new authenticated Copilot reception
claim is made.
