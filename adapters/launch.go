package adapters

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// LaunchOptions selects the host CLI protocol. Env names and ClaudeHook are
// trusted host configuration, never values from an incoming message.
type LaunchOptions struct{ Database, CLI, CLIEnv, EndpointEnv, ClaudeHook, PluginName string }

func PrepareLaunch(command []string, id int64, harnessName, launchCWD string, opts LaunchOptions) ([]string, error) {
	if opts.Database == "" || opts.Database == ":memory:" {
		return command, nil
	}
	cli := opts.CLI
	var err error
	if cli == "" {
		cli, err = os.Executable()
	}
	if err != nil {
		return nil, err
	}
	path := EndpointPath(opts.Database, id)
	env := []string{"env", opts.CLIEnv + "=" + cli, opts.EndpointEnv + "=" + path, "POKE_PROTOCOL=" + opts.CLIEnv}
	native := command
	if len(native) > 0 && native[0] == "env" {
		native = native[1:]
		for len(native) > 0 && strings.Contains(native[0], "=") {
			if !strings.HasPrefix(native[0], "POKE_PROTOCOL=") && !strings.HasPrefix(native[0], opts.CLIEnv+"=") && !strings.HasPrefix(native[0], opts.EndpointEnv+"=") {
				env = append(env, native[0])
			}
			native = native[1:]
		}
	}
	if len(native) == 0 {
		return nil, errors.New("empty messaging harness command")
	}
	switch {
	case harnessName == "codex" && native[0] == "codex":
		home := os.Getenv("CODEX_HOME")
		for _, assignment := range env {
			if value, ok := strings.CutPrefix(assignment, "CODEX_HOME="); ok {
				home = value
			}
		}
		if home == "" {
			userHome, err := os.UserHomeDir()
			if err != nil {
				return nil, err
			}
			home = filepath.Join(userHome, ".codex")
		}
		if !filepath.IsAbs(home) {
			home = filepath.Join(launchCWD, home)
		}
		nativePath := os.Getenv("PATH")
		for _, assignment := range env {
			if value, ok := strings.CutPrefix(assignment, "PATH="); ok {
				nativePath = value
			}
		}
		binary, err := recipientExecutable("codex", nativePath, launchCWD)
		if err != nil {
			return nil, err
		}
		env = append(env, "CODEX_HOME="+home)
		if err := WriteEndpoint(path, Endpoint{SessionID: id, CodexHome: home, CodexBinary: binary, NativePath: nativePath}); err != nil {
			return nil, err
		}
	case harnessName == "pi" && filepath.Base(native[0]) == "pi":
		extension := filepath.Join(filepath.Dir(path), "pi-message-extension.mjs")
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			return nil, err
		}
		if err := WritePrivateFile(extension, []byte(PiExtension)); err != nil {
			return nil, err
		}
		native = append([]string{native[0], "-e", extension}, native[1:]...)
	case harnessName == "claude" && native[0] == "claude":
		dir := filepath.Join(filepath.Dir(path), "claude-plugin")
		for _, subdir := range []string{".claude-plugin", "hooks"} {
			if err := os.MkdirAll(filepath.Join(dir, subdir), 0700); err != nil {
				return nil, err
			}
		}
		name := opts.PluginName
		if name == "" {
			name = "poke-messages"
		}
		manifestBytes, err := json.Marshal(map[string]string{"name": name, "version": "1.0.0", "description": "Poke native mailbox receiver"})
		if err != nil {
			return nil, err
		}
		manifest := string(manifestBytes)
		if err := WritePrivateFile(filepath.Join(dir, ".claude-plugin", "plugin.json"), []byte(manifest)); err != nil {
			return nil, err
		}
		hooks := map[string]any{"hooks": map[string]any{"SessionStart": []any{map[string]any{"hooks": []any{map[string]any{"type": "command", "command": opts.ClaudeHook, "timeout": 5}}}}}}
		data, _ := json.Marshal(hooks)
		if err := WritePrivateFile(filepath.Join(dir, "hooks", "hooks.json"), data); err != nil {
			return nil, err
		}
		native = append([]string{"claude", "--plugin-dir", dir}, native[1:]...)
	}
	return append(env, native...), nil
}

// Resolve the binary under the recipient's actual launch PATH, including env
// overrides, without mutating the sending process's environment.
func recipientExecutable(name, searchPath, cwd string) (string, error) {
	for _, dir := range filepath.SplitList(searchPath) {
		if dir == "" {
			dir = "."
		}
		if !filepath.IsAbs(dir) {
			dir = filepath.Join(cwd, dir)
		}
		path := filepath.Join(dir, name)
		info, err := os.Stat(path)
		if err == nil && !info.IsDir() && info.Mode().Perm()&0111 != 0 {
			return filepath.Abs(path)
		}
	}
	return "", errors.New("recipient executable unavailable in launch PATH: " + name)
}
