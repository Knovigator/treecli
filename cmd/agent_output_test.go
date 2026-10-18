package cmd

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/Knovigator/treecli/api"
	"github.com/spf13/cobra"
)

func pretendPiped(t *testing.T, piped bool) {
	t.Helper()
	previous := stdoutIsTerminal
	stdoutIsTerminal = func() bool { return !piped }
	t.Cleanup(func() { stdoutIsTerminal = previous })
	t.Setenv("TREECLI_OUTPUT", "")
}

func TestExitCodesAreStable(t *testing.T) {
	cases := map[error]int{
		nil:                                ExitOK,
		errors.New("something broke"):      ExitError,
		errors.New(`unknown flag: --nope`): ExitUsage,
		&api.HTTPError{StatusCode: 401}:    ExitNotSignedIn,
		errors.New("Treechat connection expired; run treecli connect again"):             ExitNotSignedIn,
		errors.New(`Treechat operation failed: {"error":"daily_limit_reached"}`):         ExitSpending,
		errors.New("Quoted price exceeds the accepted spending cap."):                    ExitSpending,
		&api.HTTPError{StatusCode: 404}:                                                  ExitNotFound,
		errors.New(`{"error":"rate_limit_exceeded"}`):                                    ExitRateLimited,
		&api.RequestError{StatusCode: 401, Message: "API request failed (status 401): "}: ExitNotSignedIn,
		&api.RequestError{StatusCode: 429, Message: "API request failed (status 429): "}: ExitRateLimited,
		errors.New("--out <path> is required (or use --quote to just see the price)"):    ExitUsage,
		errors.New(`invalid --payment "credits"; use usd or bsv/bitcoinsv`):              ExitUsage,
		errors.New("invalid API response: unexpected end of JSON input"):                 ExitError,
	}
	for err, want := range cases {
		if got := ExitCode(err); got != want {
			t.Errorf("ExitCode(%v) = %d, want %d", err, got, want)
		}
	}
}

func TestPipedOutputTurnsOnJSONUnlessAskedNotTo(t *testing.T) {
	previousSelection := accountSelection
	t.Cleanup(func() { accountSelection = previousSelection })
	newCommand := func() *cobra.Command {
		command := &cobra.Command{Use: "x"}
		command.Flags().Bool("json", false, "")
		return command
	}
	viperless := func(command *cobra.Command) bool {
		value, _ := command.Flags().GetBool("json")
		return value
	}

	pretendPiped(t, true)
	command := newCommand()
	_ = PrepareCommand(command, nil)
	if !viperless(command) {
		t.Fatal("piped output should default to JSON")
	}

	command = newCommand()
	_ = command.Flags().Set("json", "false")
	_ = PrepareCommand(command, nil)
	if viperless(command) {
		t.Fatal("an explicit --json=false must win")
	}

	t.Setenv("TREECLI_OUTPUT", "text")
	command = newCommand()
	_ = PrepareCommand(command, nil)
	if viperless(command) {
		t.Fatal("TREECLI_OUTPUT=text must keep text output")
	}

	pretendPiped(t, false)
	command = newCommand()
	_ = PrepareCommand(command, nil)
	if viperless(command) {
		t.Fatal("a terminal keeps text output")
	}
}

func TestExplicitOutputFormatBeatsAutomaticJSON(t *testing.T) {
	previousSelection := accountSelection
	t.Cleanup(func() { accountSelection = previousSelection })
	pretendPiped(t, true)
	command := &cobra.Command{Use: "x"}
	command.Flags().Bool("json", false, "")
	command.Flags().String("output", "", "")
	_ = command.Flags().Set("output", "ascii")
	_ = PrepareCommand(command, nil)
	if value, _ := command.Flags().GetBool("json"); value {
		t.Fatal("--output ascii must not be overridden by automatic JSON")
	}
}

func TestLegacyOnboardContentModesStayText(t *testing.T) {
	previousSelection := accountSelection
	t.Cleanup(func() { accountSelection = previousSelection })
	pretendPiped(t, true)
	command := &cobra.Command{Use: "onboard"}
	command.Flags().Bool("json", false, "")
	command.Flags().Bool("agents-md", false, "")
	_ = command.Flags().Set("agents-md", "true")
	_ = PrepareCommand(command, nil)
	if value, _ := command.Flags().GetBool("json"); value {
		t.Fatal("onboard --agents-md >> AGENTS.md must keep printing Markdown")
	}
}

func TestAccountSelectionErrorsAreUsage(t *testing.T) {
	previousSelection, previousEnv := accountSelection, SelectedEnvironment
	t.Cleanup(func() { accountSelection, SelectedEnvironment = previousSelection, previousEnv })
	command := &cobra.Command{Use: "x"}
	command.Flags().String("env", "", "")
	_ = command.Flags().Set("env", "bad!")
	SelectedEnvironment = "bad!"
	err := PrepareCommand(command, nil)
	if err == nil || ExitCode(err) != ExitUsage {
		t.Fatalf("expected a usage error, got %v (exit %d)", err, ExitCode(err))
	}
}

func TestFlagGroupErrorsAreUsage(t *testing.T) {
	err := errors.New("if any flags in the group [short long] are set none of the others can be; [long short] were all set")
	if ExitCode(err) != ExitUsage {
		t.Fatalf("got %d", ExitCode(err))
	}
}

func TestStructuredWriteErrorsCarryTheExitCode(t *testing.T) {
	var out bytes.Buffer
	PrintError(&out, &structuredWriteError{WriteID: "w-1", Message: "daily_limit_reached", Err: errors.New(`Treechat operation failed: {"error":"daily_limit_reached"}`)})
	var payload map[string]interface{}
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil || payload["write_id"] != "w-1" || payload["exit_code"] != float64(ExitSpending) {
		t.Fatalf("payload %q (%v)", out.String(), err)
	}
}

func TestErrorsAreJSONForAgents(t *testing.T) {
	pretendPiped(t, true)
	var out bytes.Buffer
	PrintError(&out, errors.New(`Treechat operation failed: {"error":"daily_limit_reached"}`))
	var payload map[string]interface{}
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatalf("not JSON: %q", out.String())
	}
	if payload["status"] != "error" || payload["exit_code"] != float64(ExitSpending) {
		t.Fatalf("payload: %v", payload)
	}

	pretendPiped(t, false)
	out.Reset()
	PrintError(&out, errors.New("boom"))
	if out.String() != "Error: boom\n" {
		t.Fatalf("people get text: %q", out.String())
	}
}

func TestConnectInstallsSkillsForDetectedAgents(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	if err := os.MkdirAll(filepath.Join(home, ".claude"), 0o755); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	ConnectCmd.SetOut(&out)
	t.Cleanup(func() { ConnectCmd.SetOut(nil); connectInstallSkills = false })

	if err := offerAgentSkills(ConnectCmd); err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(out.Bytes(), []byte("treecli skills install all --claude")) {
		t.Fatalf("expected a hint, got %q", out.String())
	}

	connectInstallSkills = true
	out.Reset()
	if err := offerAgentSkills(ConnectCmd); err != nil {
		t.Fatal(err)
	}
	entries, _ := os.ReadDir(filepath.Join(home, ".claude", "skills"))
	if len(entries) == 0 {
		t.Fatalf("no skills installed; output %q", out.String())
	}
}
