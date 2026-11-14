package local

import (
	"context"
	"database/sql"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/carlotran4/poke/adapters"
	"github.com/carlotran4/poke/mailbox"
)

func fixture(t *testing.T) (*Store, context.Context, Endpoint, Endpoint) {
	t.Helper()
	ctx := context.Background()
	s, err := Open(filepath.Join(t.TempDir(), "mail.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	a, err := s.Register(ctx, "builder", "manual")
	if err != nil {
		t.Fatal(err)
	}
	b, err := s.Register(ctx, "reviewer", "manual")
	if err != nil {
		t.Fatal(err)
	}
	return s, ctx, a, b
}
func TestDurableExchangeAndAuthorization(t *testing.T) {
	s, ctx, a, b := fixture(t)
	m, err := s.Send(ctx, a.ID, b.ID, "one", "review please", 0)
	if err != nil {
		t.Fatal(err)
	}
	same, err := s.Send(ctx, a.ID, b.ID, "one", "review please", 0)
	if err != nil || same.ID != m.ID {
		t.Fatal(same, err)
	}
	if _, err = s.Send(ctx, a.ID, b.ID, "one", "changed", 0); err == nil {
		t.Fatal("conflicting key accepted")
	}
	if _, err = s.Send(ctx, a.ID, a.ID, "self", "x", 0); err == nil {
		t.Fatal("self send")
	}
	if _, err = s.Send(ctx, a.ID, 999, "missing", "x", 0); err == nil {
		t.Fatal("missing endpoint")
	}
	if _, err = s.Send(ctx, a.ID, 0, "bad-reply", "x", m.ID); err == nil {
		t.Fatal("sender replied as recipient")
	}
	if err = mailbox.Acknowledge(ctx, s.DB, a.ID, m.ID); err == nil {
		t.Fatal("sender acknowledged recipient's mail")
	}
	if err = s.Drain(ctx, b.ID); err != nil {
		t.Fatal(err)
	}
	saved, _ := mailbox.Get(ctx, s.DB, m.ID)
	if saved.State != "queued" || saved.NotificationStatus != "deferred" {
		t.Fatal(saved)
	}
	if err = mailbox.ReceiptMessageRead(ctx, s.DB, b.ID, m.ID); err != nil {
		t.Fatal(err)
	}
	if err = mailbox.Acknowledge(ctx, s.DB, b.ID, m.ID); err != nil {
		t.Fatal(err)
	}
	if err = mailbox.ReceiptMessageRead(ctx, s.DB, b.ID, m.ID); err != nil {
		t.Fatal(err)
	}
	saved, _ = mailbox.Get(ctx, s.DB, m.ID)
	if saved.State != "acknowledged" {
		t.Fatal("receipt regressed ack", saved)
	}
	reply, err := s.Send(ctx, b.ID, 0, "reply", "reviewed", m.ID)
	if err != nil || reply.Recipient != a.ID || reply.Depth != 1 {
		t.Fatal(reply, err)
	}
	if err = s.Retire(ctx, b.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Send(ctx, b.ID, a.ID, "retired", "x", 0); err == nil {
		t.Fatal("retired sender")
	}
	if _, err = s.Send(ctx, a.ID, b.ID, "queued", "retired recipient retains mail", 0); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Register(ctx, b.Name, "manual"); err == nil {
		t.Fatal("retired name reused")
	}
	other, err := Open(s.Path)
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	history, err := mailbox.Inbox(ctx, other.DB, b.ID, true)
	if err != nil || len(history) != 2 {
		t.Fatal(history, err)
	}
}
func TestConcurrentClaimsRestartAndRetry(t *testing.T) {
	s, ctx, a, b := fixture(t)
	other, err := Open(s.Path)
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	m, err := s.Send(ctx, a.ID, b.ID, "one", "work", 0)
	if err != nil {
		t.Fatal(err)
	}
	var count atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			db := s.DB
			if i%2 == 0 {
				db = other.DB
			}
			ok, err := mailbox.Claim(ctx, db, b.ID, m.ID, "fake", s.Guard())
			if err != nil {
				t.Error(err)
			}
			if ok {
				count.Add(1)
			}
		}(i)
	}
	wg.Wait()
	if count.Load() != 1 {
		t.Fatal(count.Load())
	}
	if err = mailbox.RetryMessageNotification(ctx, other.DB, a.ID, m.ID); err == nil {
		t.Fatal("retried in flight")
	}
	m2, err := s.Send(ctx, a.ID, b.ID, "two", "next", 0)
	if err != nil {
		t.Fatal(err)
	}
	if ok, err := mailbox.Claim(ctx, other.DB, b.ID, m2.ID, "fake", s.Guard()); err != nil || ok {
		t.Fatal("uncertain fence lost", ok, err)
	}
	_, err = s.DB.ExecContext(ctx, `update session_messages set notification_attempted_at=? where id=?`, time.Now().UTC().Add(-time.Minute), m.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err = mailbox.RetryMessageNotification(ctx, s.DB, 999, m.ID); err == nil {
		t.Fatal("stranger retried")
	}
	if err = mailbox.RetryMessageNotification(ctx, s.DB, a.ID, m.ID); err != nil {
		t.Fatal(err)
	}
	if ok, err := mailbox.Claim(ctx, s.DB, b.ID, m.ID, "fake", s.Guard()); err != nil || !ok {
		t.Fatal(ok, err)
	}
	if err = mailbox.RecordMessageNotificationSubmitted(ctx, s.DB, b.ID, m.ID); err != nil {
		t.Fatal(err)
	}
	if err = mailbox.ReceiptMessageRead(ctx, s.DB, b.ID, m.ID); err != nil {
		t.Fatal(err)
	}
	if ok, err := mailbox.Claim(ctx, other.DB, b.ID, m2.ID, "fake", s.Guard()); err != nil || !ok {
		t.Fatal("receipt did not release fence", ok, err)
	}
}
func TestConcurrentSendDeduplicatesAndBoundsReplies(t *testing.T) {
	s, ctx, a, b := fixture(t)
	other, err := Open(s.Path)
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	var wg sync.WaitGroup
	ids := make(chan int64, 10)
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			host := s
			if i%2 == 0 {
				host = other
			}
			m, err := host.Send(ctx, a.ID, b.ID, "same", "same", 0)
			if err != nil {
				t.Error(err)
				return
			}
			ids <- m.ID
		}(i)
	}
	wg.Wait()
	close(ids)
	var first int64
	for id := range ids {
		if first == 0 {
			first = id
		}
		if id != first {
			t.Fatal("duplicate durable messages")
		}
	}
	m, err := mailbox.Get(ctx, s.DB, first)
	if err != nil {
		t.Fatal(err)
	}
	for depth := 1; depth <= 8; depth++ {
		m, err = s.Send(ctx, m.Recipient, 0, time.Now().String(), "reply", m.ID)
		if err != nil {
			t.Fatal(depth, err)
		}
	}
	if _, err = s.Send(ctx, m.Recipient, 0, "nine", "reply", m.ID); err == nil {
		t.Fatal("unbounded reply chain")
	}
}
func TestBindingsCannotRetargetAndRetireFencesClaims(t *testing.T) {
	s, ctx, a, _ := fixture(t)
	var err error
	if err = s.Start(ctx, a.ID, t.TempDir()); err != nil {
		t.Fatal(err)
	}
	if err = s.Bind(ctx, a.ID, adapters.Endpoint{NativeRef: "one"}); err != nil {
		t.Fatal(err)
	}
	if err = s.Bind(ctx, a.ID, adapters.Endpoint{NativeRef: "two"}); err == nil {
		t.Fatal("conversation retargeted")
	}
	if _, err = s.Native(ctx, a.ID); err == nil {
		t.Fatal("disabled receiver still active")
	}
	if err = s.Bind(ctx, a.ID, adapters.Endpoint{NativeRef: "one"}); err == nil {
		t.Fatal("disabled binding revived")
	}
	if err = s.Start(ctx, a.ID, "other"); err == nil {
		t.Fatal("attempt launched twice")
	}
}
func TestStandaloneRefusesHostDatabaseAndNewerSchema(t *testing.T) {
	path := filepath.Join(t.TempDir(), "host.db")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.DB.Exec(`update poke_schema set version=2`)
	if err != nil {
		t.Fatal(err)
	}
	s.Close()
	if next, err := Open(path); err == nil {
		next.Close()
		t.Fatal("newer schema accepted")
	}
	s, ctx, _, _ := fixture(t)
	_, err = s.DB.ExecContext(ctx, `drop table poke_schema`)
	if err != nil {
		t.Fatal(err)
	}
	if next, err := Open(s.Path); err == nil {
		next.Close()
		t.Fatal("host mailbox adopted")
	}
}
func TestSendAuthorizationRollback(t *testing.T) {
	s, ctx, a, b := fixture(t)
	_, err := mailbox.Send(ctx, s.DB, func(context.Context, *sql.Tx, int64, int64) error { return context.Canceled }, a.ID, b.ID, "denied", "x", 0)
	if err == nil {
		t.Fatal("denied send committed")
	}
	ms, err := mailbox.Inbox(ctx, s.DB, b.ID, true)
	if err != nil || len(ms) != 0 {
		t.Fatal(ms, err)
	}
}

func TestSendTxCommitsWithHostStateOrRollsBackTogether(t *testing.T) {
	s, ctx, a, b := fixture(t)
	if _, err := s.DB.Exec(`create table host_events(id integer primary key)`); err != nil {
		t.Fatal(err)
	}
	auth := func(ctx context.Context, tx *sql.Tx, a, b int64) error { return nil }
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(`insert into host_events values(1)`); err != nil {
		t.Fatal(err)
	}
	m, err := mailbox.SendTx(ctx, tx, auth, a.ID, b.ID, "rollback", "x", 0)
	if err != nil {
		t.Fatal(err)
	}
	if err = tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	if _, err = mailbox.Get(ctx, s.DB, m.ID); err != sql.ErrNoRows {
		t.Fatal("mail survived rollback", err)
	}
	tx, err = s.DB.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(`insert into host_events values(2)`); err != nil {
		t.Fatal(err)
	}
	m, err = mailbox.SendTx(ctx, tx, auth, a.ID, b.ID, "commit", "x", 0)
	if err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if _, err = mailbox.Get(ctx, s.DB, m.ID); err != nil {
		t.Fatal(err)
	}
	var n int
	if err = s.DB.QueryRow(`select count(*) from host_events`).Scan(&n); err != nil || n != 1 {
		t.Fatal(n, err)
	}
}
