package adapters

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// Keep one env prefix so Herdr still recognizes canonical agent executables.
// Replace inherited identities, including wrapper-provided stale values.
func WithIdentity(command []string, sessionID int64, dbPath, sessionEnv, dbEnv string) []string {
	result := []string{"env", fmt.Sprintf("%s=%d", sessionEnv, sessionID), dbEnv + "=" + dbPath}
	if len(command) > 0 && command[0] == "env" {
		command = command[1:]
		for len(command) > 0 && strings.Contains(command[0], "=") {
			if !strings.HasPrefix(command[0], sessionEnv+"=") && !strings.HasPrefix(command[0], dbEnv+"=") {
				result = append(result, command[0])
			}
			command = command[1:]
		}
	}
	// Codex may run tools in its daemon, whose inherited environment belongs to
	// another client. Pin identity in this thread's explicit shell policy as well.
	// Global -c options precede resume subcommands and positional ticket prompts.
	if len(command) > 0 && command[0] == "codex" {
		sessionValue, _ := json.Marshal(strconv.FormatInt(sessionID, 10))
		dbValue, _ := json.Marshal(dbPath)
		result = append(result, "codex", "-c", "shell_environment_policy.set."+sessionEnv+"="+string(sessionValue), "-c", "shell_environment_policy.set."+dbEnv+"="+string(dbValue))
		return append(result, command[1:]...)
	}
	return append(result, command...)
}
