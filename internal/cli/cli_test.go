package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"testing"
)

func TestCLIExchange(t *testing.T) {
	t.Setenv("POKE_DB", filepath.Join(t.TempDir(), "mail.db"))
	t.Setenv("POKE_SESSION_ID", "")
	call := func(args ...string) map[string]any {
		t.Helper()
		var out, stderr bytes.Buffer
		if err := Run(context.Background(), args, bytes.NewReader(nil), &out, &stderr); err != nil {
			t.Fatal(args, err)
		}
		var result map[string]any
		if err := json.Unmarshal(out.Bytes(), &result); err != nil {
			t.Fatal(out.String(), err)
		}
		return result
	}
	a := call("register", "builder")
	b := call("register", "reviewer")
	t.Setenv("POKE_SESSION_ID", fmt.Sprint(a["id"]))
	m := call("send", "reviewer", "--key", "one", "--body", "review please")
	id := fmt.Sprint(m["id"])
	if m["state"] != "queued" {
		t.Fatal(m)
	}
	var out, stderr bytes.Buffer
	if err := Run(context.Background(), []string{"ack", id}, bytes.NewReader(nil), &out, &stderr); err == nil {
		t.Fatal("sender acknowledged")
	}
	t.Setenv("POKE_SESSION_ID", fmt.Sprint(b["id"]))
	if err := Run(context.Background(), []string{"inbox"}, bytes.NewReader(nil), &out, &stderr); err != nil {
		t.Fatal(err)
	}
	if got := call("ack", id); got["state"] != "acknowledged" {
		t.Fatal(got)
	}
	reply := call("reply", id, "--key", "reply", "--body", "reviewed")
	if reply["recipient"] != a["id"] {
		t.Fatal(reply)
	}
	call("retire", "reviewer")
	if got := call("status", id); got["state"] != "acknowledged" {
		t.Fatal(got)
	}
}
func TestHelpHasNoSideEffects(t *testing.T) {
	t.Setenv("POKE_DB", "/unwritable/not-created/mail.db")
	var out bytes.Buffer
	if err := Run(context.Background(), []string{"--help"}, nil, &out, &out); err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(out.Bytes(), []byte("poke send NAME")) {
		t.Fatal(out.String())
	}
}
