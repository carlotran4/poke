# Delivery semantics

Poke separates a message's durable state from the evidence about its wake notice.

| Message state | Meaning |
| --- | --- |
| queued | Stored; reception has not been established |
| notified | Native acceptance or recipient body fetch established reception |
| acknowledged | Recipient explicitly accepted responsibility |

| Notification status | Meaning and retry policy |
| --- | --- |
| pending | Committed and eligible for an attempt |
| deferred | No submission; host may retry when eligibility/capability improves |
| uncertain | Claim committed before side effect; acceptance may have happened |
| submitted | Socket/terminal submission completed; reception still unknown |
| notified | Native acceptance or body-fetch evidence |

One uncertain/submitted handoff per recipient fences other unconfirmed wakes.
A crash cannot turn uncertainty back into a silent retry. Read or native receipts
release subsequent handoffs without waiting for acknowledgment or task completion.
Pi can reconcile a persisted native custom message after interruption.

Only the recipient may reply or acknowledge. Replies derive their destination
from the parent, and chains stop after eight hops. Sender-scoped keys deduplicate
identical sends; reusing one for different content or routing is an error. Hosts
must enforce participant authorization when exposing exact-ID status queries.

`inbox` reads are bounded to the oldest 100 records and never acknowledge work.
The CLI records body-fetch receipts for returned records. `inbox --all` includes
acknowledged history but is still bounded; it is not a complete export API.
Libraries can read in a host transaction to compose receipts with other state.

An explicit retry is available only to a participant for an uncertain queued
notice older than 30 seconds. It preserves the message ID but may duplicate a wake
whose prior acceptance was unknown. Submitted notices wait for recipient evidence;
they are not silently replayed. No component sends automatic replies or treats
acknowledgment as proof that a task succeeded.

Standalone attempts are immutable and never reused. Retired recipients retain
mail and can inspect it through their old environment identity, but cannot send new
mail. Retired native receivers cannot claim notifications. The same OS account can
impersonate those identities, so this is lifecycle protection, not hostile-user auth.
