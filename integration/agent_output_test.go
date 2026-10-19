package integration_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// With stdout piped and no TREECLI_OUTPUT override, errors are JSON on stderr
// and the process exits with the stable code for the failure.
func TestCompiledAgentOutputErrorsAndExitCodes(t *testing.T) {
	binary := buildCLI(t, repositoryRoot(t))
	configHome := t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, binary, "get", "threads", "--no-such-flag")
	environment := []string{}
	for _, entry := range isolatedEnvironment(configHome) {
		if !strings.HasPrefix(entry, "TREECLI_OUTPUT=") {
			environment = append(environment, entry)
		}
	}
	cmd.Env = environment
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	err := cmd.Run()

	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) || exitErr.ExitCode() != 2 {
		t.Fatalf("expected usage exit code 2, got %v (stderr %q)", err, stderr.String())
	}
	var payload map[string]interface{}
	if json.Unmarshal([]byte(strings.TrimSpace(stderr.String())), &payload) != nil || payload["status"] != "error" {
		t.Fatalf("expected a JSON error on stderr, got %q", stderr.String())
	}
}
