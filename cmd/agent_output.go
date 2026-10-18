package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/term"
)

// Stable exit codes, so agents and scripts can branch without parsing text.
const (
	ExitOK          = 0
	ExitError       = 1
	ExitUsage       = 2
	ExitNotSignedIn = 3 // run treecli connect (or login)
	ExitSpending    = 4 // price cap, daily spending limit, or billing
	ExitNotFound    = 5
	ExitRateLimited = 6
)

// usageError marks an invalid invocation (exit code 2) without changing its message.
type usageError struct{ err error }

func (e usageError) Error() string { return e.err.Error() }
func (e usageError) Unwrap() error { return e.err }

// usageErrorf reports an invalid invocation: a bad or missing argument or flag.
func usageErrorf(format string, args ...interface{}) error {
	return usageError{fmt.Errorf(format, args...)}
}

// stdoutIsTerminal is a variable so tests can pretend output is piped.
var stdoutIsTerminal = func() bool { return term.IsTerminal(int(os.Stdout.Fd())) }

// agentOutput reports whether output should be machine-readable: when stdout
// isn't a terminal (an agent or script is reading it), unless TREECLI_OUTPUT=text.
func agentOutput() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("TREECLI_OUTPUT"))) {
	case "text":
		return false
	case "json":
		return true
	}
	return !stdoutIsTerminal()
}

// PrepareCommand runs before every command: it turns on --json for commands
// that have it when output is piped, then applies account selection.
func PrepareCommand(command *cobra.Command, args []string) error {
	if agentOutput() {
		// An explicit text format wins over the automatic JSON: --output (e.g.
		// ascii), or onboard's legacy content modes, which print Markdown to append
		// to a file (onboard --agents-md >> AGENTS.md).
		explicitFormat := false
		for _, name := range []string{"output", "agents-md", "short", "long"} {
			if flag := command.Flags().Lookup(name); flag != nil && flag.Changed {
				explicitFormat = true
			}
		}
		if flag := command.Flags().Lookup("json"); flag != nil && !flag.Changed && !explicitFormat && flag.Value.Type() == "bool" {
			_ = command.Flags().Set("json", "true")
		}
	}
	// Everything account selection rejects is an invalid --env/--account/--profile.
	if err := ConfigureAccountSelection(command, args); err != nil {
		return usageError{err}
	}
	return nil
}

// ExitCode maps an error to one of the stable exit codes.
func ExitCode(err error) int {
	if err == nil {
		return ExitOK
	}
	var usage usageError
	if errors.As(err, &usage) {
		return ExitUsage
	}
	var statusErr interface{ HTTPStatus() int }
	if errors.As(err, &statusErr) {
		switch statusErr.HTTPStatus() {
		case 401, 403:
			return ExitNotSignedIn
		case 402:
			return ExitSpending
		case 404:
			return ExitNotFound
		case 429:
			return ExitRateLimited
		}
	}
	message := strings.ToLower(err.Error())
	contains := func(parts ...string) bool {
		for _, part := range parts {
			if strings.Contains(message, part) {
				return true
			}
		}
		return false
	}
	switch {
	case contains("daily_limit_reached", "price_cap_exceeded", "price cap", "spending cap", "spending limit", "insufficient", "billing is not", "payment required"):
		return ExitSpending
	case contains("rate_limit_exceeded", "rate limit", "too many requests", "try again later"):
		return ExitRateLimited
	case contains("missing credentials", "treecli connect", "connection expired", "invalid_token", "not signed in", "log in again", "login changed"):
		return ExitNotSignedIn
	case contains("not_found", "not found"):
		return ExitNotFound
	// Cobra's own flag and argument errors are untyped; the server reports bad
	// tool arguments as invalid_arguments. treecli's own checks use usageErrorf.
	case contains("unknown command", "unknown flag", "unknown shorthand flag", "required flag(s)",
		"accepts ", "requires at least", "invalid argument", "flag needs an argument",
		"if any flags in the group", "at least one of the flags in the group", "invalid_arguments"):
		return ExitUsage
	}
	return ExitError
}

// PrintError reports err on stderr: JSON for agents (with the exit code), text
// for people. Structured write errors keep their write ID either way.
func PrintError(writer io.Writer, err error) {
	var structuredErr *structuredWriteError
	if errors.As(err, &structuredErr) {
		encoder := json.NewEncoder(writer)
		encoder.SetIndent("", "  ")
		if encoder.Encode(writeErrorPayload{
			Status:   "error",
			WriteID:  structuredErr.WriteID,
			Error:    structuredErr.Message,
			ExitCode: ExitCode(err),
		}) == nil {
			return
		}
	}
	if agentOutput() {
		if data, marshalErr := json.Marshal(map[string]interface{}{
			"status":    "error",
			"error":     err.Error(),
			"exit_code": ExitCode(err),
		}); marshalErr == nil {
			fmt.Fprintln(writer, string(data))
			return
		}
	}
	fmt.Fprintln(writer, "Error:", err)
}
