// Package adapters implements bounded native notifications. The host validates
// conversation ownership and claims durable delivery BEFORE calling a transport.
package adapters

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type Endpoint struct {
	CodexBinary string `json:"codex_binary,omitempty"`
	NativePath  string `json:"native_path,omitempty"`
	SessionID   int64  `json:"session_id"`
	NativeRef   string `json:"native_ref"`
	Socket      string `json:"socket,omitempty"`
	Token       string `json:"token,omitempty"`
	CodexHome   string `json:"codex_home,omitempty"`
	Disabled    bool   `json:"disabled,omitempty"`
}

func EndpointPath(dbPath string, id int64) string {
	path, _ := filepath.Abs(dbPath)
	if real, err := filepath.EvalSymlinks(path); err == nil {
		path = real
	}
	return filepath.Join(path+".messages", strconv.FormatInt(id, 10)+".json")
}

func WriteEndpoint(path string, endpoint Endpoint) error {
	data, err := json.Marshal(endpoint)
	if err != nil {
		return err
	}
	return WritePrivateFile(path, data)
}
func WritePrivateFile(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	info, err := os.Lstat(dir)
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode().Perm()&0077 != 0 {
		return errors.New("messaging directory must be private")
	}
	file, err := os.CreateTemp(dir, ".message-asset-")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	if _, err = file.Write(data); err != nil {
		file.Close()
		return err
	}
	if err = file.Close(); err != nil {
		return err
	}
	return os.Rename(file.Name(), path)
}

func ReadEndpoint(path string) (Endpoint, error) {
	var endpoint Endpoint
	info, err := os.Lstat(path)
	if err != nil {
		return endpoint, err
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 || info.Size() > 65536 {
		return endpoint, errors.New("messaging endpoint must be a private regular file")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return endpoint, err
	}
	err = json.Unmarshal(data, &endpoint)
	return endpoint, err
}

type NotSentError struct{ error }

func PostClaudeNotice(ctx context.Context, endpoint Endpoint, text string) error {
	path := strings.TrimPrefix(endpoint.Socket, "uds:")
	connection, err := (&net.Dialer{}).DialContext(ctx, "unix", path)
	if err != nil {
		return NotSentError{err}
	}
	defer connection.Close()
	deadline := time.Now().Add(time.Second)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	_ = connection.SetDeadline(deadline)
	writer := bufio.NewWriter(connection)
	encoder := json.NewEncoder(writer)
	if endpoint.Token != "" {
		if err = encoder.Encode(map[string]string{"type": "auth", "token": endpoint.Token}); err != nil {
			return err
		}
	}
	if err = encoder.Encode(map[string]any{"type": "user", "from": "poke", "message": map[string]string{"role": "user", "content": text}}); err != nil {
		return err
	}
	return writer.Flush() // A write is submitted, not model acceptance; inbox read receipts it.
}

// PostCodexNotice uses the recipient's recorded executable, home, PATH and cwd.
// A command error is ambiguous and must not be automatically retried.
func PostCodexNotice(ctx context.Context, e Endpoint, cwd, ref, notice string) error {
	if e.CodexBinary == "" || e.CodexHome == "" || ref == "" {
		return NotSentError{errors.New("verified Codex endpoint required")}
	}
	cmd := exec.CommandContext(ctx, e.CodexBinary, "queue", "--thread", ref, "--message", notice)
	cmd.Dir = cwd
	cmd.Env = append(cmd.Environ(), "CODEX_HOME="+e.CodexHome, "PATH="+e.NativePath)
	return cmd.Run()
}
