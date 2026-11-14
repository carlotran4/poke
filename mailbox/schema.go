package mailbox

// Schema is the v1 mailbox schema. No endpoint foreign keys: retiring a host
// endpoint must not delete another participant's correspondence.
const Schema = `create table if not exists session_messages (
 id integer primary key autoincrement,
 sender integer not null,
 recipient integer not null,
 request_key text not null,
 body text not null,
 reply_to integer not null default 0,
 depth integer not null default 0 check(depth between 0 and 8),
 state text not null default 'queued' check(state in ('queued','notified','acknowledged')),
 created_at datetime not null,
 notified_at datetime,
 acknowledged_at datetime,
 notification_status text not null default 'pending',
 notification_transport text not null default '',
 notification_error text not null default '',
 notification_attempted_at datetime,
 unique(sender, request_key)
);
create index if not exists session_messages_inbox on session_messages(recipient,state,id);`
