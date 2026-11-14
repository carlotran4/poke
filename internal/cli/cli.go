// Package cli implements the versioned JSON command surface.
package cli

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"time"

	"github.com/carlotran4/poke/adapters"
	"github.com/carlotran4/poke/local"
	"github.com/carlotran4/poke/mailbox"
)

const Help = `poke — durable messaging for coding agents

Usage:
  poke init
  poke register NAME [--harness manual|pi|claude|codex]
  poke recipients
  poke launch NAME -- HARNESS [ARGS...]
  poke retire NAME
  poke send NAME --key KEY --body TEXT
  poke inbox [--all]
  poke ack MESSAGE_ID
  poke reply MESSAGE_ID --key KEY --body TEXT
  poke status MESSAGE_ID
  poke retry MESSAGE_ID
  poke drain [NAME]

Native receiver commands (run in the owning launched agent):
  poke register-claude
  poke bind-codex VERIFIED_REF
  poke pending | claim ID | defer-native ID | notified ID

Environment:
  POKE_DB          SQLite path (default: user data directory/poke/poke.db)
  POKE_SESSION_ID  Sender/recipient attempt ID; injected by poke launch

Output is JSON except help and version. Errors are JSON on stderr.
Mail is committed before bounded notification. No daemon or automatic resume.
`

func flags(name string, args []string, setup func(*flag.FlagSet)) error {
	f := flag.NewFlagSet(name, flag.ContinueOnError)
	f.SetOutput(io.Discard)
	setup(f)
	if err := f.Parse(args); err != nil {
		return err
	}
	if f.NArg() != 0 {
		return errors.New("unexpected positional arguments")
	}
	return nil
}
func positive(v string) (int64, error) {
	id, err := strconv.ParseInt(v, 10, 64)
	if err != nil || id <= 0 {
		return 0, errors.New("positive message ID required")
	}
	return id, nil
}
func Run(ctx context.Context, args []string, in io.Reader, out, stderr io.Writer) error {
	if len(args) == 0 || args[0] == "help" || args[0] == "--help" || args[0] == "-h" {
		_, err := io.WriteString(out, Help)
		return err
	}
	if args[0] == "version" {
		return json.NewEncoder(out).Encode(map[string]any{"version": "0.1.0", "protocol": 1, "schema": 1})
	}
	path := os.Getenv("POKE_DB")
	if path == "" {
		base := os.Getenv("XDG_DATA_HOME")
		if base == "" {
			home, err := os.UserHomeDir()
			if err != nil {
				return err
			}
			base = filepath.Join(home, ".local", "share")
		}
		path = filepath.Join(base, "poke", "poke.db")
	}
	s, err := local.Open(path)
	if err != nil {
		return err
	}
	defer s.Close()
	encode := json.NewEncoder(out).Encode
	switch args[0] {
	case "init":
		if len(args) != 1 {
			return errors.New("usage: poke init")
		}
		return encode(map[string]any{"database": s.Path, "schema": 1})
	case "register":
		if len(args) < 2 {
			return errors.New("usage: poke register NAME [--harness NAME]")
		}
		h := "manual"
		err = flags("register", args[2:], func(f *flag.FlagSet) { f.StringVar(&h, "harness", h, "") })
		if err != nil {
			return err
		}
		e, err := s.Register(ctx, args[1], h)
		if err != nil {
			return err
		}
		return encode(e)
	case "recipients":
		if len(args) != 1 {
			return errors.New("usage: poke recipients")
		}
		e, err := s.Recipients(ctx)
		if err != nil {
			return err
		}
		return encode(e)
	case "retire":
		if len(args) != 2 {
			return errors.New("usage: poke retire NAME")
		}
		e, err := s.Resolve(ctx, args[1])
		if err != nil {
			return err
		}
		if err = s.Retire(ctx, e.ID); err != nil {
			return err
		}
		return encode(map[string]bool{"retired": true})
	case "launch":
		return launch(ctx, s, args, in, out, stderr)
	case "drain":
		if len(args) > 2 {
			return errors.New("usage: poke drain [NAME]")
		}
		es, err := s.Recipients(ctx)
		if err != nil {
			return err
		}
		if len(args) == 2 {
			e, err := s.Resolve(ctx, args[1])
			if err != nil {
				return err
			}
			es = []local.Endpoint{e}
		}
		for _, e := range es {
			op, cancel := context.WithTimeout(ctx, 2*time.Second)
			err = s.Drain(op, e.ID)
			cancel()
			if err != nil {
				return err
			}
		}
		return encode(map[string]bool{"drained": true})
	}
	sender, err := local.Identity()
	if err != nil {
		return err
	}
	endpoint, err := s.ByID(ctx, sender)
	if err != nil {
		return err
	}
	notify := func(id int64) {
		op, cancel := context.WithTimeout(ctx, 2*time.Second)
		defer cancel()
		_ = s.Drain(op, id)
	}
	switch args[0] {
	case "send", "reply":
		if len(args) < 2 {
			return errors.New("send needs a recipient name; reply needs a message ID")
		}
		var key, body string
		if err = flags(args[0], args[2:], func(f *flag.FlagSet) { f.StringVar(&key, "key", "", ""); f.StringVar(&body, "body", "", "") }); err != nil {
			return err
		}
		var recipient, reply int64
		if args[0] == "send" {
			e, err := s.Resolve(ctx, args[1])
			if err != nil {
				return err
			}
			recipient = e.ID
		} else {
			reply, err = positive(args[1])
			if err != nil {
				return err
			}
		}
		m, err := s.Send(ctx, sender, recipient, key, body, reply)
		if err != nil {
			return err
		}
		notify(m.Recipient)
		if updated, e := mailbox.Get(ctx, s.DB, m.ID); e == nil {
			m = updated
		}
		return encode(m)
	case "inbox", "pending":
		all := len(args) == 2 && args[1] == "--all"
		if len(args) != 1 && (!all || args[0] != "inbox") {
			return errors.New("usage: poke inbox [--all] or poke pending")
		}
		if args[0] == "pending" {
			if endpoint.Harness != "pi" {
				return errors.New("pending requires a Pi receiver")
			}
			if _, err = s.Native(ctx, sender); err != nil {
				return err
			}
		}
		ms, err := mailbox.Inbox(ctx, s.DB, sender, all)
		if err != nil {
			return err
		}
		if args[0] == "inbox" {
			for i, m := range ms {
				if err = mailbox.ReceiptMessageRead(ctx, s.DB, sender, m.ID); err != nil {
					return err
				}
				ms[i], err = mailbox.Get(ctx, s.DB, m.ID)
				if err != nil {
					return err
				}
			}
			notify(sender)
		}
		return encode(ms)
	case "register-claude":
		if len(args) != 1 || endpoint.Harness != "claude" {
			return errors.New("register-claude requires the owning Claude attempt")
		}
		var event struct {
			SessionID string `json:"session_id"`
		}
		if err = json.NewDecoder(io.LimitReader(in, 65536)).Decode(&event); err != nil {
			return err
		}
		socket := os.Getenv("CLAUDE_CODE_MESSAGING_SOCKET")
		if socket == "" {
			return errors.New("Claude native inbox socket is unavailable")
		}
		err = s.Bind(ctx, sender, adapters.Endpoint{NativeRef: event.SessionID, Socket: socket, Token: os.Getenv("CLAUDE_CODE_MESSAGING_TOKEN")})
		if err != nil {
			return err
		}
		return encode(map[string]bool{"registered": true})
	case "bind-codex":
		if len(args) != 2 || args[1] == "" || endpoint.Harness != "codex" {
			return errors.New("usage: poke bind-codex VERIFIED_REF (owning Codex attempt only)")
		}
		n, err := adapters.ReadEndpoint(adapters.EndpointPath(s.Path, sender))
		if err != nil {
			return err
		}
		n.NativeRef = args[1]
		if err = s.Bind(ctx, sender, n); err != nil {
			return err
		}
		return encode(map[string]bool{"registered": true})
	case "ack", "status", "retry", "claim", "defer-native", "notified":
		if len(args) != 2 {
			return errors.New("one message ID required")
		}
		id, err := positive(args[1])
		if err != nil {
			return err
		}
		m, err := mailbox.Get(ctx, s.DB, id)
		if err != nil {
			return err
		}
		if m.Sender != sender && m.Recipient != sender {
			return errors.New("message does not belong to this endpoint")
		}
		if args[0] == "status" {
			return encode(m)
		}
		if args[0] == "retry" {
			if err = mailbox.RetryMessageNotification(ctx, s.DB, sender, id); err != nil {
				return err
			}
			notify(m.Recipient)
		} else {
			if m.Recipient != sender {
				return errors.New("only the recipient may update delivery state")
			}
			if args[0] == "ack" {
				err = mailbox.Acknowledge(ctx, s.DB, sender, id)
				if err == nil {
					notify(sender)
				}
			} else {
				if endpoint.Harness != "pi" {
					return errors.New("native claims require the owning Pi receiver")
				}
				unlock, e := s.Lock(ctx)
				if e != nil {
					return e
				}
				defer unlock()
				if _, err = s.Native(ctx, sender); err != nil {
					return err
				}
				switch args[0] {
				case "claim":
					ok, err := mailbox.Claim(ctx, s.DB, sender, id, "pi_extension", s.Guard())
					if err != nil {
						return err
					}
					return encode(map[string]bool{"claimed": ok})
				case "defer-native":
					err = mailbox.DeferNativeMessage(ctx, s.DB, sender, id)
				case "notified":
					err = mailbox.Notified(ctx, s.DB, sender, id, s.Guard())
				}
			}
			if err != nil {
				return err
			}
		}
		m, err = mailbox.Get(ctx, s.DB, id)
		if err != nil {
			return err
		}
		return encode(m)
	default:
		return fmt.Errorf("unknown command %q; run poke --help", args[0])
	}
}
func launch(ctx context.Context, s *local.Store, args []string, in io.Reader, out, stderr io.Writer) error {
	if len(args) < 4 || args[2] != "--" {
		return errors.New("usage: poke launch NAME -- HARNESS [ARGS...]")
	}
	e, err := s.Resolve(ctx, args[1])
	if err != nil {
		return err
	}
	if e.Harness == "manual" || args[3] != e.Harness {
		return errors.New("launch requires a matching registered native harness")
	}
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	cli, err := os.Executable()
	if err != nil {
		return err
	}
	if err = s.Start(ctx, e.ID, cwd); err != nil {
		return err
	}
	defer s.Retire(context.Background(), e.ID)
	command, err := adapters.PrepareLaunch(args[3:], e.ID, e.Harness, cwd, adapters.LaunchOptions{Database: s.Path, CLI: cli, CLIEnv: "POKE_CLI", EndpointEnv: "POKE_ENDPOINT_FILE", ClaudeHook: `"$POKE_CLI" register-claude`})
	if err != nil {
		return err
	}
	command = adapters.WithIdentity(command, e.ID, s.Path, "POKE_SESSION_ID", "POKE_DB")
	cmd := exec.CommandContext(ctx, command[0], command[1:]...)
	cmd.Stdin = in
	cmd.Stdout = out
	cmd.Stderr = stderr
	return cmd.Run()
}
