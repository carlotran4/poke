package mailbox

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// Guard is a host-authored SQL predicate appended inside an atomic update.
// Never construct SQL from agent input. Args must match the predicate's placeholders.
// Hosts must also synchronize transport submission with endpoint close/rebind.
type Guard struct {
	SQL  string
	Args []any
}

func guard(g Guard) (string, error) {
	if g.SQL == "" {
		return "", errors.New("delivery eligibility guard is required")
	}
	return " and (" + g.SQL + ")", nil
}

// Claim fences competing notifiers before any transport side effect. Unknown
// outcomes remain uncertain across process restarts; they are never auto-replayed.
func Claim(ctx context.Context, db DBTX, recipient, id int64, transport string, g Guard) (bool, error) {
	suffix, err := guard(g)
	if err != nil {
		return false, err
	}
	args := []any{transport, time.Now().UTC(), id, recipient, recipient}
	args = append(args, g.Args...)
	res, err := db.ExecContext(ctx, `update session_messages set notification_status='uncertain',notification_transport=?,notification_error='notification in flight or acceptance unknown',notification_attempted_at=?
 where id=? and recipient=? and state='queued' and notification_status in ('pending','deferred')
 and not exists(select 1 from session_messages other where other.recipient=? and other.id<>session_messages.id and other.state<>'acknowledged' and other.notification_status in ('uncertain','submitted'))`+suffix, args...)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n == 1, err
}

// Notified records positive native acceptance, never task completion.
func Notified(ctx context.Context, db DBTX, recipient, id int64, g Guard) error {
	suffix, err := guard(g)
	if err != nil {
		return err
	}
	args := []any{time.Now().UTC(), id, recipient}
	args = append(args, g.Args...)
	res, err := db.ExecContext(ctx, `update session_messages set state=case when state='queued' then 'notified' else state end,notified_at=coalesce(notified_at,?),notification_status='notified',notification_error='' where id=? and recipient=?`+suffix, args...)
	return requireAffected(res, err)
}
func requireAffected(res sql.Result, err error) error {
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err == nil && n != 1 {
		return sql.ErrNoRows
	}
	return err
}
func Acknowledge(ctx context.Context, db DBTX, recipient, id int64) error {
	res, err := db.ExecContext(ctx, `update session_messages set state='acknowledged',acknowledged_at=coalesce(acknowledged_at,?),notification_status='notified',notification_error='' where id=? and recipient=?`, time.Now().UTC(), id, recipient)
	return requireAffected(res, err)
}

func DeferMessageNotification(ctx context.Context, db DBTX, recipient, id int64, transport, reason string) error {
	_, err := db.ExecContext(ctx, `update session_messages set notification_status='deferred',notification_transport=?,notification_error=? where id=? and recipient=? and state='queued' and notification_status in ('pending','deferred')`, transport, reason, id, recipient)
	return err
}
func RecordMessageNotificationError(ctx context.Context, db DBTX, recipient, id int64, reason string) error {
	_, err := db.ExecContext(ctx, `update session_messages set notification_error=? where id=? and recipient=? and state='queued' and notification_status='uncertain'`, reason, id, recipient)
	return err
}

func RetryMessageNotification(ctx context.Context, db DBTX, participant, id int64) error {
	res, err := db.ExecContext(ctx, `update session_messages set notification_status='pending',notification_error='explicit retry of uncertain notification' where id=? and (sender=? or recipient=?) and state='queued' and notification_status='uncertain' and notification_attempted_at<?`, id, participant, participant, time.Now().UTC().Add(-30*time.Second))
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return errors.New("only an uncertain queued notification older than 30 seconds can be explicitly retried")
	}
	return nil
}

func RecordMessageNotificationSubmitted(ctx context.Context, db DBTX, recipient, id int64) error {
	_, err := db.ExecContext(ctx, `update session_messages set notification_status='submitted',notification_error='' where id=? and recipient=? and state='queued' and notification_status='uncertain'`, id, recipient)
	return err
}

func ReceiptMessageRead(ctx context.Context, db DBTX, recipient, id int64) error {
	_, err := db.ExecContext(ctx, `update session_messages set state=case when state='queued' then 'notified' else state end,notified_at=coalesce(notified_at,?),notification_status='notified',notification_error='' where id=? and recipient=? and state<>'acknowledged'`, time.Now().UTC(), id, recipient)
	return err
}
func DeferNativeMessage(ctx context.Context, db DBTX, recipient, id int64) error {
	_, err := db.ExecContext(ctx, `update session_messages set notification_status='deferred',notification_error='native input state changed before submission' where id=? and recipient=? and state='queued' and notification_status='uncertain' and notification_transport='pi_extension'`, id, recipient)
	return err
}

func ReleaseUnsentNotification(ctx context.Context, db DBTX, recipient, id int64, reason string) error {
	_, err := db.ExecContext(ctx, `update session_messages set notification_status='deferred',notification_error=? where id=? and recipient=? and state='queued' and notification_status='uncertain'`, reason, id, recipient)
	return err
}
