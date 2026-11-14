# Host integration

Poke separates durable mail, native transport and host lifecycle policy.
The standalone CLI uses `local`; an embedding metaharness normally uses `mailbox`
and `adapters` directly. Kanbi is a production consumer of that boundary.

## SQLite and transactions

`mailbox.Schema` is the v1 schema for new databases. Apply it from your migration
transaction; do not execute arbitrary migrations on an existing host database.
The historical table name `session_messages` preserves compatibility with Kanbi.
Hosts with older schemas own upgrading the four notification columns before using
this version. Poke does not open, close or migrate an embedded host's database.

Use WAL and a busy timeout on every connection. `mailbox.Send` opens and commits its
own transaction. Its required `AuthorizeSend` callback validates sender and resolved
recipient through that same `*sql.Tx` after the write lock is acquired. Replies
resolve their destination before authorization. Identical retry keys return the
existing row, even if the sender later closed; changing payload/routing is rejected.

Use `mailbox.SendTx` when committing mail together with other host state. The caller
owns commit/rollback and must reserve the write lock before any earlier reads that
might otherwise need a SQLite read-to-write upgrade. `SendTx` never notifies or
commits. Delivery/receipt APIs accept either `*sql.DB` or `*sql.Tx` via `DBTX`.

The schema has no endpoint foreign keys. Retiring or deleting an endpoint must
not erase another participant's correspondence. Endpoint IDs are nonzero signed
64-bit host identifiers scoped to one database. The standalone registry maps names
to positive immutable IDs; Kanbi preserves positive attempt IDs and negative
Operator addresses. Numeric values carry no global meaning. Hosts own namespaces,
identity injection, participant access to status, and discovery.

## Delivery is an atomic lifecycle decision

1. Hold the host's cross-process lifecycle lock.
2. Resolve the current attempt, verified native ref and recipient environment.
3. Validate the exact runtime container and native receiver binding.
4. Call `mailbox.Claim` with a host-authored `Guard`. Its SQL predicate runs in the
   same update that claims the message, using the `session_messages` row.
5. Submit a fixed routing notice through a native adapter, within a deadline.
6. Record the result, even if the original caller's deadline has expired.
7. Release the lifecycle lock.

`Guard.SQL` is trusted host code, never agent-supplied SQL. Bind dynamic values
through `Guard.Args`. An empty guard is rejected. For example, the standalone
host uses `exists(select 1 from poke_endpoints e where
 e.id=session_messages.recipient and e.active=1)`.

The SQL guard protects durable eligibility. The lifecycle lock protects the gap
between checking the process and submitting to it. Both are needed. Rebinding a
stable address must invalidate old process generations; stale processes cannot be
allowed to use the new mailbox identity. The standalone host avoids rebinding
entirely and requires a fresh name for every attempt.

`adapters.PostClaudeNotice` returns `NotSentError` only when connecting failed
before any write. A partial write or failed Codex command is ambiguous. Release a
claim only with proof no submission occurred. Preserve uncertainty otherwise.
Native acceptance or fetching inbox bodies releases the next handoff; acknowledgment
is separate responsibility evidence.

## Launch and receiver setup

`adapters.PrepareLaunch` installs the Pi extension or scoped Claude plugin and
records Codex's recipient environment. Supply your absolute CLI path, environment
names and Claude registration hook. `adapters.WithIdentity` replaces stale identity
assignments and pins canonical Codex identity into thread-local shell policy.

Launch preparation is for canonical native commands, not arbitrary wrapper
inference. Pi also accepts a direct path named `pi`; Claude/Codex setup requires
`claude`/`codex` as the canonical executable token. Custom wrappers must integrate
explicitly. The standalone launch command accepts canonical command names.

Pi's bundled receiver supports the Poke CLI protocol and the Kanbi `messages`
protocol. `POKE_PROTOCOL` explicitly selects the launch host, preventing inherited
identity from another host from winning. Old Kanbi native entries remain valid
receipt evidence after upgrading. The extension rechecks native conversation,
idle/pending-turn state and editor contents after asynchronous work before sending.

Never copy socket credentials into durable messages, portable backups or logs.
The host must handle conversation changes, endpoint retirement, and version
capability errors explicitly. Poke never substitutes terminal input automatically.

## Host-controlled wake content

The durable model retains an optional `wake_prompt` field for a host to attach
standing-instruction reload/routing instructions to its native receiver response.
Treat this as trusted host output; never copy an incoming message body there.
Kanbi uses it for Operator instruction reload. Poke's own notices carry IDs and
inbox instructions only, leaving bodies in SQLite until the receiver fetches them.

## Terminal fallback

Poke does not include a standalone terminal injector. A host that chooses a
Copilot fallback must validate its own container, gate busy/permission state,
claim first, perform one literal paste plus Enter, and record only `submitted`.
Draft preservation is not guaranteed. Kanbi retains this host-specific adapter.
