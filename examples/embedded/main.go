// A small host using Poke's standalone registry as its endpoint repository.
// A full metaharness can instead use mailbox with its own SQLite transactions.
package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/carlotran4/poke/local"
	"github.com/carlotran4/poke/mailbox"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run() error {
	root, err := os.MkdirTemp("", "poke-example-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(root)
	s, err := local.Open(filepath.Join(root, "mail.db"))
	if err != nil {
		return err
	}
	defer s.Close()
	ctx := context.Background()
	builder, err := s.Register(ctx, "builder", "manual")
	if err != nil {
		return err
	}
	reviewer, err := s.Register(ctx, "reviewer", "manual")
	if err != nil {
		return err
	}
	m, err := s.Send(ctx, builder.ID, reviewer.ID, "review-v1", "Ready for review.", 0)
	if err != nil {
		return err
	}
	if err = mailbox.ReceiptMessageRead(ctx, s.DB, reviewer.ID, m.ID); err != nil {
		return err
	}
	if err = mailbox.Acknowledge(ctx, s.DB, reviewer.ID, m.ID); err != nil {
		return err
	}
	reply, err := s.Send(ctx, reviewer.ID, 0, "reviewed-v1", "Reviewed.", m.ID)
	if err != nil {
		return err
	}
	fmt.Printf("message %d acknowledged; reply %d routed to builder\n", m.ID, reply.ID)
	return nil
}
