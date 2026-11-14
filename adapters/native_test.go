package adapters

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestClaudeFramingAndUnsentClassification(t *testing.T) {
	dir, err := os.MkdirTemp("/tmp", "poke-sock-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	path := filepath.Join(dir, "inbox.sock")
	ln, err := net.Listen("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	frames := make(chan []map[string]any, 1)
	go func() {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		defer c.Close()
		c.SetDeadline(time.Now().Add(time.Second))
		d := json.NewDecoder(c)
		var auth, msg map[string]any
		if d.Decode(&auth) != nil || d.Decode(&msg) != nil {
			return
		}
		frames <- []map[string]any{auth, msg}
	}()
	if err = PostClaudeNotice(context.Background(), Endpoint{Socket: path, Token: "secret"}, "fixed notice"); err != nil {
		t.Fatal(err)
	}
	select {
	case f := <-frames:
		if f[0]["token"] != "secret" || f[1]["type"] != "user" || f[1]["message"].(map[string]any)["content"] != "fixed notice" {
			t.Fatal(f)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no frames")
	}
	err = PostClaudeNotice(context.Background(), Endpoint{Socket: path + "missing"}, "notice")
	var unsent NotSentError
	if !errors.As(err, &unsent) {
		t.Fatal("connection failure not classified as unsent", err)
	}
}
func TestCodexRecipientEnvironmentAndArguments(t *testing.T) {
	dir := t.TempDir()
	binary := filepath.Join(dir, "codex")
	log := filepath.Join(dir, "log")
	script := "#!/bin/sh\nprintf '%s\\n' \"$CODEX_HOME\" \"$PATH\" \"$PWD\" \"$@\" > \"$POKE_TEST_LOG\"\n"
	if err := os.WriteFile(binary, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CODEX_HOME", "sender-must-not-leak")
	t.Setenv("POKE_TEST_LOG", log)
	err := PostCodexNotice(context.Background(), Endpoint{CodexBinary: binary, CodexHome: dir + "/home", NativePath: "/recipient/bin"}, dir, "thread-ref", "literal $(do not execute)")
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	want := strings.Join([]string{dir + "/home", "/recipient/bin", dir, "queue", "--thread", "thread-ref", "--message", "literal $(do not execute)", ""}, "\n")
	if string(data) != want {
		t.Fatalf("%q != %q", data, want)
	}
}
func TestPrivateAssetsAndIdentity(t *testing.T) {
	path := EndpointPath(filepath.Join(t.TempDir(), "mail.db"), 4)
	if err := WriteEndpoint(path, Endpoint{SessionID: 4, NativeRef: "ref"}); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{path, filepath.Dir(path)} {
		info, err := os.Stat(p)
		if err != nil || info.Mode().Perm()&0077 != 0 {
			t.Fatal(info, err)
		}
	}
	if err := os.Chmod(path, 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadEndpoint(path); err == nil {
		t.Fatal("public endpoint read")
	}
	input := []string{"env", "POKE_SESSION_ID=old", "POKE_DB=old", "TOKEN=kept", "codex", "resume", "ref"}
	got := WithIdentity(input, 4, "/tmp/mail.db", "POKE_SESSION_ID", "POKE_DB")
	want := []string{"env", "POKE_SESSION_ID=4", "POKE_DB=/tmp/mail.db", "TOKEN=kept", "codex", "-c", `shell_environment_policy.set.POKE_SESSION_ID="4"`, "-c", `shell_environment_policy.set.POKE_DB="/tmp/mail.db"`, "resume", "ref"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("%q != %q", got, want)
	}
	if input[1] != "POKE_SESSION_ID=old" {
		t.Fatal("mutated command")
	}
}
func TestLaunchNativeAssetsAndProtocolIsolation(t *testing.T) {
	for _, h := range []string{"pi", "claude", "codex"} {
		t.Run(h, func(t *testing.T) {
			dir := t.TempDir()
			if h == "codex" {
				if err := os.WriteFile(filepath.Join(dir, "codex"), []byte("#!/bin/sh\nexit 0\n"), 0700); err != nil {
					t.Fatal(err)
				}
				t.Setenv("PATH", dir+":"+os.Getenv("PATH"))
			}
			cmd, err := PrepareLaunch([]string{"env", "POKE_PROTOCOL=stale", "EXISTING=yes", h, "--option"}, 7, h, dir, LaunchOptions{Database: filepath.Join(dir, "mail.db"), CLI: "/bin/poke", CLIEnv: "POKE_CLI", EndpointEnv: "POKE_ENDPOINT_FILE", ClaudeHook: `"$POKE_CLI" register-claude`})
			if err != nil {
				t.Fatal(err)
			}
			joined := strings.Join(cmd, " ")
			if !strings.Contains(joined, "POKE_PROTOCOL=POKE_CLI") || strings.Contains(joined, "stale") || !strings.Contains(joined, "EXISTING=yes") || !strings.Contains(joined, "--option") {
				t.Fatal(cmd)
			}
			if h == "pi" && !strings.Contains(joined, " -e ") {
				t.Fatal(cmd)
			}
			if h == "claude" && !strings.Contains(joined, " --plugin-dir ") {
				t.Fatal(cmd)
			}
		})
	}
}

func TestLaunchRecordsExecutableFromRecipientPathOverride(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "codex")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\nexit 0\n"), 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "mail.db")
	_, err := PrepareLaunch([]string{"env", "PATH=" + dir, "codex"}, 4, "codex", dir, LaunchOptions{Database: path, CLI: "/bin/poke", CLIEnv: "POKE_CLI", EndpointEnv: "POKE_ENDPOINT_FILE"})
	if err != nil {
		t.Fatal(err)
	}
	e, err := ReadEndpoint(EndpointPath(path, 4))
	if err != nil || e.CodexBinary != bin || e.NativePath != dir {
		t.Fatal(e, err)
	}
}
