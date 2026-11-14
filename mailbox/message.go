// Package mailbox provides durable local agent messaging on host-owned SQLite.
// Hosts retain lifecycle ownership. Calls never launch an agent or wait for a model.
package mailbox

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
)

// DBTX is implemented by *sql.DB and *sql.Tx. The caller owns their lifetime.
type DBTX interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

// AuthorizeSend runs under the same SQLite write transaction as insertion.
// It must validate both endpoints using tx, not another database connection.
type AuthorizeSend func(context.Context, *sql.Tx, int64, int64) error

type Message struct {
	WakePrompt              string     `json:"wake_prompt,omitempty"`
	NotificationStatus      string     `json:"notification_status"`
	NotificationTransport   string     `json:"notification_transport"`
	NotificationError       string     `json:"notification_error,omitempty"`
	NotificationAttemptedAt *time.Time `json:"notification_attempted_at"`
	ID                      int64      `json:"id"`
	Sender                  int64      `json:"sender"`
	Recipient               int64      `json:"recipient"`
	RequestKey              string     `json:"request_key"`
	Body                    string     `json:"body"`
	ReplyTo                 int64      `json:"reply_to,omitempty"`
	Depth                   int        `json:"depth"`
	State                   string     `json:"state"`
	CreatedAt               time.Time  `json:"created_at"`
	NotifiedAt              *time.Time `json:"notified_at"`
	AcknowledgedAt          *time.Time `json:"acknowledged_at"`
}

const messageSelect = `select id,sender,recipient,request_key,body,reply_to,depth,state,created_at,notified_at,acknowledged_at,notification_status,notification_transport,notification_error,notification_attempted_at from session_messages`

type messageScanner interface{ Scan(...any) error }

func scanMessage(row messageScanner) (m Message, err error) {
	err = row.Scan(&m.ID, &m.Sender, &m.Recipient, &m.RequestKey, &m.Body, &m.ReplyTo, &m.Depth, &m.State, &m.CreatedAt, &m.NotifiedAt, &m.AcknowledgedAt, &m.NotificationStatus, &m.NotificationTransport, &m.NotificationError, &m.NotificationAttemptedAt)
	return
}

func Get(ctx context.Context, db DBTX, id int64) (Message, error) {
	return scanMessage(db.QueryRowContext(ctx, messageSelect+` where id=?`, id))
}

// Send commits before any notification. A retry must reuse its
// sender-scoped key and identical content. Replies derive their destination from
// the parent, never from caller-supplied addressing.
func Send(ctx context.Context, db *sql.DB, authorize AuthorizeSend, sender, recipient int64, key, body string, replyTo int64) (Message, error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return Message{}, err
	}
	defer tx.Rollback()
	m, err := SendTx(ctx, tx, authorize, sender, recipient, key, body, replyTo)
	if err != nil {
		return Message{}, err
	}
	return m, tx.Commit()
}

// SendTx composes message insertion with host work in the same transaction.
// It acquires the SQLite write lock before reads; caller commits or rolls back.
// Notification must happen only AFTER the caller commits successfully.
func SendTx(ctx context.Context, tx *sql.Tx, authorize AuthorizeSend, sender, recipient int64, key, body string, replyTo int64) (Message, error) {
	if sender == 0 || strings.TrimSpace(key) == "" || len(key) > 128 || strings.TrimSpace(body) == "" || len(body) > 16384 || replyTo < 0 {
		return Message{}, errors.New("sender, request key (1–128 bytes), and body (1–16384 bytes) are required")
	}
	if authorize == nil {
		return Message{}, errors.New("send authorization is required")
	}
	// Reserve the write lock before reading, avoiding read-to-write lock upgrades.
	if _, err := tx.ExecContext(ctx, `update session_messages set state=state where id=-1`); err != nil {
		return Message{}, err
	}
	depth := 0
	if replyTo != 0 {
		parent, e := scanMessage(tx.QueryRowContext(ctx, messageSelect+` where id=?`, replyTo))
		if e != nil {
			return Message{}, e
		}
		if parent.Recipient != sender {
			return Message{}, errors.New("only the recipient may reply")
		}
		recipient = parent.Sender
		depth = parent.Depth + 1
		if depth > 8 {
			return Message{}, errors.New("reply limit reached (8 hops); start a human-reviewed thread")
		}
	}
	if recipient == 0 || sender == recipient {
		return Message{}, errors.New("a different recipient is required")
	}
	existing, e := scanMessage(tx.QueryRowContext(ctx, messageSelect+` where sender=? and request_key=?`, sender, key))
	if e == nil {
		if existing.Recipient != recipient || existing.Body != body || existing.ReplyTo != replyTo {
			return Message{}, errors.New("request key already used with different content")
		}
		return existing, nil
	}
	if !errors.Is(e, sql.ErrNoRows) {
		return Message{}, e
	}
	if authorize == nil {
		return Message{}, errors.New("send authorization is required")
	}
	if err := authorize(ctx, tx, sender, recipient); err != nil {
		return Message{}, err
	}
	result, err := tx.ExecContext(ctx, `insert into session_messages(sender,recipient,request_key,body,reply_to,depth,created_at) values(?,?,?,?,?,?,?)`, sender, recipient, key, body, replyTo, depth, time.Now().UTC())
	if err != nil {
		return Message{}, err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return Message{}, err
	}
	m, err := scanMessage(tx.QueryRowContext(ctx, messageSelect+` where id=?`, id))
	if err != nil {
		return Message{}, err
	}
	return m, nil
}

// Inbox is non-consuming; notification is not acknowledgement.
func Inbox(ctx context.Context, db DBTX, recipient int64, includeAcknowledged bool) ([]Message, error) {
	query := messageSelect + ` where recipient=?`
	if !includeAcknowledged {
		query += ` and state<>'acknowledged'`
	}
	rows, err := db.QueryContext(ctx, query+` order by id limit 100`, recipient)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	messages := []Message{}
	for rows.Next() {
		m, e := scanMessage(rows)
		if e != nil {
			return nil, e
		}
		messages = append(messages, m)
	}
	return messages, rows.Err()
}
