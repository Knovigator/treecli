package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/Knovigator/treecli/recorder"
	"github.com/spf13/cobra"
)

// McpCmd groups the agent-recording surface: an MCP server, hook handlers,
// per-agent installers, and maintenance commands.
var McpCmd = &cobra.Command{
	Use:   "mcp",
	Short: "Record Claude Code, Codex and Grok sessions into memdb and Treechat; serve them back over MCP",
	Long: `treecli mcp turns treecli into a plugin for AI coding agents.

  treecli mcp install --claude --codex --grok   register the MCP server + hooks with each agent
  treecli mcp serve --agent claude-code         the stdio MCP server the agents launch
  treecli mcp hook --agent claude-code          the hook handler the agents run (reads JSON on stdin)
  treecli mcp backfill --claude --codex         ingest transcripts that already exist on this machine
  treecli mcp status                            what is configured and recorded
  treecli mcp config --treechat-mode session --treechat-team <stream id>

Sessions are stored as memdb ingest files under the treecli data directory and
ingested into memdb-rust (SQLite + FTS5, optional Qdrant hybrid search). With a
Treechat mode set, each session also becomes a thread in the chosen stream.`,
}

var mcpServeCmd = &cobra.Command{
	Use:   "serve",
	Short: "Run the treecli MCP server over stdio (agents launch this)",
	Args:  cobra.NoArgs,
	RunE:  runMcpServe,
}

var mcpHookCmd = &cobra.Command{
	Use:   "hook",
	Short: "Handle one agent hook event (JSON on stdin); always exits 0",
	Args:  cobra.NoArgs,
	RunE:  runMcpHook,
}

var mcpStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show recorder configuration, installed agents and recent sessions",
	Args:  cobra.NoArgs,
	RunE:  runMcpStatus,
}

var mcpConfigCmd = &cobra.Command{
	Use:   "config",
	Short: "Set recorder options (memdb paths, Treechat stream and mode, recall behaviour)",
	Args:  cobra.NoArgs,
	RunE:  runMcpConfig,
}

var mcpBackfillCmd = &cobra.Command{
	Use:   "backfill",
	Short: "Ingest Claude Code and Codex transcripts already on this machine into memdb (never posts to Treechat)",
	Args:  cobra.NoArgs,
	RunE:  runMcpBackfill,
}

var mcpRecordCmd = &cobra.Command{
	Use:   "record <transcript>",
	Short: "Ingest one transcript into memdb (and Treechat with --post)",
	Args:  cobra.ExactArgs(1),
	RunE:  runMcpRecord,
}

var (
	mcpAgentFlag          string
	mcpHookEventFlag      string
	mcpHookVerbose        bool
	mcpStatusJSON         bool
	mcpConfigMemdbBinFlag string
	mcpConfigMemdbDBVal   string
	mcpConfigDataDirVal   string
	mcpConfigTeam         string
	mcpConfigMode         string
	mcpConfigEnv          string
	mcpConfigAccount      string
	mcpConfigStart        string
	mcpConfigPrompt       string
	mcpConfigShow         bool
	mcpBackfillClaude     bool
	mcpBackfillCodex      bool
	mcpBackfillAll        bool
	mcpBackfillSince      string
	mcpBackfillLimit      int
	mcpBackfillDryRun     bool
	mcpRecordPost         bool
	mcpRecordEnded        bool
)

func init() {
	mcpServeCmd.Flags().StringVar(&mcpAgentFlag, "agent", "", "Agent this server is attached to: claude, codex, or grok (default: $TREECLI_AGENT)")
	mcpHookCmd.Flags().StringVar(&mcpAgentFlag, "agent", "", "Agent that fired the hook: claude, codex, or grok (default: $TREECLI_AGENT)")
	mcpHookCmd.Flags().StringVar(&mcpHookEventFlag, "event", "", "Event name when the payload does not carry one (SessionStart, UserPromptSubmit, PostToolUse, Stop, SessionEnd)")
	mcpHookCmd.Flags().BoolVar(&mcpHookVerbose, "verbose", false, "Also print handler errors to stderr")
	mcpStatusCmd.Flags().BoolVar(&mcpStatusJSON, "json", false, "Print machine-readable status")
	mcpConfigCmd.Flags().StringVar(&mcpConfigMemdbBinFlag, "memdb-bin", "", "Path to the memdb-rust binary")
	mcpConfigCmd.Flags().StringVar(&mcpConfigMemdbDBVal, "memdb-db", "", "Path to the memdb SQLite database")
	mcpConfigCmd.Flags().StringVar(&mcpConfigDataDirVal, "data-dir", "", "Directory for recordings and state")
	mcpConfigCmd.Flags().StringVar(&mcpConfigTeam, "treechat-team", "", "Treechat stream (team) id or link that session threads are posted into")
	mcpConfigCmd.Flags().StringVar(&mcpConfigMode, "treechat-mode", "", "Treechat recording: off, session (open + close posts), or turns (also one reply per turn)")
	mcpConfigCmd.Flags().StringVar(&mcpConfigEnv, "treechat-env", "", "Environment whose saved account posts (default: the usual selection)")
	mcpConfigCmd.Flags().StringVar(&mcpConfigAccount, "treechat-account", "", "Saved account that posts (default: the environment's active account)")
	mcpConfigCmd.Flags().StringVar(&mcpConfigStart, "recall-on-start", "", "true/false: inject recent project history at SessionStart")
	mcpConfigCmd.Flags().StringVar(&mcpConfigPrompt, "recall-on-prompt", "", "true/false: inject matching durable memories on each prompt")
	mcpConfigCmd.Flags().BoolVar(&mcpConfigShow, "show", false, "Print the effective configuration")
	mcpBackfillCmd.Flags().BoolVar(&mcpBackfillClaude, "claude", false, "Backfill Claude Code transcripts (~/.claude/projects)")
	mcpBackfillCmd.Flags().BoolVar(&mcpBackfillCodex, "codex", false, "Backfill Codex rollouts (~/.codex/sessions)")
	mcpBackfillCmd.Flags().BoolVar(&mcpBackfillAll, "all", false, "Backfill every supported agent")
	mcpBackfillCmd.Flags().StringVar(&mcpBackfillSince, "since", "", "Only transcripts modified within this window (e.g. 30d, 12h) or after this date (YYYY-MM-DD)")
	mcpBackfillCmd.Flags().IntVar(&mcpBackfillLimit, "limit", 0, "Stop after this many transcripts (newest first)")
	mcpBackfillCmd.Flags().BoolVar(&mcpBackfillDryRun, "dry-run", false, "List what would be ingested without writing")
	mcpRecordCmd.Flags().StringVar(&mcpAgentFlag, "agent", "", "Transcript format: claude or codex (required)")
	mcpRecordCmd.Flags().BoolVar(&mcpRecordPost, "post", false, "Also mirror the session into Treechat per the configured mode")
	mcpRecordCmd.Flags().BoolVar(&mcpRecordEnded, "ended", false, "Treat the session as finished (posts the closing summary with --post)")

	McpCmd.AddCommand(mcpServeCmd)
	McpCmd.AddCommand(mcpHookCmd)
	McpCmd.AddCommand(mcpInstallCmd)
	McpCmd.AddCommand(mcpUninstallCmd)
	McpCmd.AddCommand(mcpStatusCmd)
	McpCmd.AddCommand(mcpConfigCmd)
	McpCmd.AddCommand(mcpBackfillCmd)
	McpCmd.AddCommand(mcpRecordCmd)
}

func resolveMcpAgent(flag string) (recorder.Agent, error) {
	value := firstNonEmpty(flag, os.Getenv("TREECLI_AGENT"))
	if value == "" {
		return "", errors.New("--agent is required (claude, codex, or grok), or set TREECLI_AGENT")
	}
	return recorder.ParseAgent(value)
}

func runMcpServe(cmd *cobra.Command, args []string) error {
	agent, err := resolveMcpAgent(mcpAgentFlag)
	if err != nil {
		return err
	}
	settings, err := loadMcpSettings()
	if err != nil {
		return err
	}
	rec, err := buildRecorder(settings, false)
	if err != nil {
		return err
	}
	cwd, _ := os.Getwd()
	server := recorder.NewMCPServer(recorder.MCPServerOptions{Agent: agent, CWD: cwd, Recorder: rec, Version: CurrentVersion})
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	rec.Store.AppendLog("mcp serve started agent=%s cwd=%s memdb=%s", agent, cwd, settings.MemdbBin)
	if err := recorder.RunStdio(ctx, server); err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, io.EOF) {
		rec.Store.AppendLog("mcp serve exited: %v", err)
		return err
	}
	return nil
}

func runMcpHook(cmd *cobra.Command, args []string) error {
	// Hooks must never fail the agent: every problem is logged and the
	// process still exits 0.
	agent, err := resolveMcpAgent(mcpAgentFlag)
	if err != nil {
		fmt.Fprintln(cmd.ErrOrStderr(), "treecli mcp hook:", err)
		return nil
	}
	settings, err := loadMcpSettings()
	if err != nil {
		fmt.Fprintln(cmd.ErrOrStderr(), "treecli mcp hook:", err)
		return nil
	}
	rec, err := buildRecorder(settings, false)
	if err != nil {
		fmt.Fprintln(cmd.ErrOrStderr(), "treecli mcp hook:", err)
		return nil
	}
	payload, err := io.ReadAll(io.LimitReader(cmd.InOrStdin(), 32*1024*1024))
	if err != nil {
		rec.Store.AppendLog("%s hook: reading stdin: %v", agent, err)
		return nil
	}
	event, err := recorder.ParseHookPayload(payload, mcpHookEventFlag)
	if err != nil {
		rec.Store.AppendLog("%s hook: %v", agent, err)
		return nil
	}
	handler := &recorder.HookHandler{
		Agent:    agent,
		Recorder: rec,
		Options:  recorder.HookOptions{RecallOnStart: settings.RecallOnStart, RecallOnPrompt: settings.RecallOnPrompt, MaxContextChars: recorder.DefaultHookOptions.MaxContextChars},
		Stdout:   cmd.OutOrStdout(),
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	if err := handler.Handle(ctx, event); err != nil {
		rec.Store.AppendLog("%s %s session=%s: %v", agent, event.Name, event.SessionID, err)
		if mcpHookVerbose {
			fmt.Fprintln(cmd.ErrOrStderr(), "treecli mcp hook:", err)
		}
	}
	return nil
}

type mcpStatusReport struct {
	DataDir        string                 `json:"data_dir"`
	MemdbBinary    string                 `json:"memdb_binary,omitempty"`
	MemdbError     string                 `json:"memdb_error,omitempty"`
	MemdbDB        string                 `json:"memdb_db"`
	MemdbDBExists  bool                   `json:"memdb_db_exists"`
	Treechat       map[string]string      `json:"treechat"`
	RecallOnStart  bool                   `json:"recall_on_start"`
	RecallOnPrompt bool                   `json:"recall_on_prompt"`
	Agents         map[string]agentStatus `json:"agents"`
	Sessions       []recorder.Marker      `json:"recent_sessions"`
	Recordings     int                    `json:"recordings"`
}

func runMcpStatus(cmd *cobra.Command, args []string) error {
	settings, err := loadMcpSettings()
	if err != nil {
		return err
	}
	store, err := recorder.NewStore(settings.DataDir)
	if err != nil {
		return err
	}
	report := mcpStatusReport{
		DataDir:        settings.DataDir,
		MemdbBinary:    settings.MemdbBin,
		MemdbDB:        settings.MemdbDB,
		RecallOnStart:  settings.RecallOnStart,
		RecallOnPrompt: settings.RecallOnPrompt,
		Treechat:       map[string]string{"mode": settings.TreechatMode, "team_id": settings.TreechatTeamID},
		Agents:         map[string]agentStatus{},
	}
	if settings.MemdbBinErr != nil {
		report.MemdbError = settings.MemdbBinErr.Error()
	}
	if _, err := os.Stat(settings.MemdbDB); err == nil {
		report.MemdbDBExists = true
	}
	for _, agent := range recorder.KnownAgents {
		report.Agents[string(agent)] = inspectAgentInstall(agent)
	}
	markers, err := store.ListMarkers()
	if err == nil {
		if len(markers) > 8 {
			markers = markers[:8]
		}
		report.Sessions = markers
	}
	report.Recordings = countRecordings(store)
	if mcpStatusJSON {
		encoder := json.NewEncoder(cmd.OutOrStdout())
		encoder.SetIndent("", "  ")
		return encoder.Encode(report)
	}
	out := cmd.OutOrStdout()
	fmt.Fprintf(out, "Data dir:    %s\n", report.DataDir)
	if report.MemdbBinary != "" {
		fmt.Fprintf(out, "memdb:       %s\n", report.MemdbBinary)
	} else {
		fmt.Fprintf(out, "memdb:       NOT FOUND (%s)\n", report.MemdbError)
	}
	dbState := "not created yet"
	if report.MemdbDBExists {
		dbState = "exists"
	}
	fmt.Fprintf(out, "memdb db:    %s (%s)\n", report.MemdbDB, dbState)
	fmt.Fprintf(out, "Treechat:    mode=%s team=%s\n", settings.TreechatMode, firstNonEmpty(settings.TreechatTeamID, "(private threads)"))
	fmt.Fprintf(out, "Recall:      on-start=%t on-prompt=%t\n", settings.RecallOnStart, settings.RecallOnPrompt)
	fmt.Fprintln(out, "Agents:")
	for _, agent := range recorder.KnownAgents {
		status := report.Agents[string(agent)]
		fmt.Fprintf(out, "  %-12s mcp=%-5t hooks=%-5t %s\n", recorder.AgentLabel(agent), status.MCP, status.Hooks, status.Detail)
	}
	fmt.Fprintf(out, "Recordings:  %d\n", report.Recordings)
	if len(report.Sessions) > 0 {
		fmt.Fprintln(out, "Recent sessions:")
		for _, marker := range report.Sessions {
			state := "active"
			if marker.Ended {
				state = "ended"
			}
			fmt.Fprintf(out, "  %s %-12s %s turns=%d %s (%s)\n", marker.UpdatedAt.Local().Format("Jan 2 15:04"), marker.Agent, marker.SessionID, marker.Turns, marker.CWD, state)
		}
	}
	return nil
}

func countRecordings(store *recorder.Store) int {
	count := 0
	for _, agent := range recorder.KnownAgents {
		entries, err := os.ReadDir(store.RecordingPath(agent, "x")[:len(store.RecordingPath(agent, "x"))-len("x.jsonl")])
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".jsonl") {
				count++
			}
		}
	}
	return count
}

func parseBoolFlag(name, value string) (bool, error) {
	parsed, err := strconv.ParseBool(strings.TrimSpace(value))
	if err != nil {
		return false, fmt.Errorf("--%s must be true or false", name)
	}
	return parsed, nil
}

func runMcpConfig(cmd *cobra.Command, args []string) error {
	values := map[string]interface{}{}
	if mcpConfigMemdbBinFlag != "" {
		if _, err := recorder.ResolveMemdbBinary(mcpConfigMemdbBinFlag); err != nil {
			return fmt.Errorf("--memdb-bin %s is not an executable file", mcpConfigMemdbBinFlag)
		}
		values[mcpConfigMemdbBin] = mcpConfigMemdbBinFlag
	}
	if mcpConfigMemdbDBVal != "" {
		values[mcpConfigMemdbDB] = mcpConfigMemdbDBVal
	}
	if mcpConfigDataDirVal != "" {
		values[mcpConfigDataDir] = mcpConfigDataDirVal
	}
	if mcpConfigTeam != "" {
		teamID, err := normalizeTeamTarget(mcpConfigTeam)
		if err != nil {
			return err
		}
		values[mcpConfigTreechatTeam] = teamID
	}
	if mcpConfigMode != "" {
		mode, err := recorder.ParseTreechatMode(mcpConfigMode)
		if err != nil {
			return err
		}
		values[mcpConfigTreechatMode] = mode
	}
	if mcpConfigEnv != "" {
		values[mcpConfigTreechatEnv] = normalizeProfileName(mcpConfigEnv)
	}
	if mcpConfigAccount != "" {
		values[mcpConfigTreechatAcct] = normalizeProfileName(mcpConfigAccount)
	}
	if mcpConfigStart != "" {
		value, err := parseBoolFlag("recall-on-start", mcpConfigStart)
		if err != nil {
			return err
		}
		values[mcpConfigRecallOnStart] = value
	}
	if mcpConfigPrompt != "" {
		value, err := parseBoolFlag("recall-on-prompt", mcpConfigPrompt)
		if err != nil {
			return err
		}
		values[mcpConfigRecallOnPrompt] = value
	}
	if len(values) == 0 && !mcpConfigShow {
		return errors.New("nothing to set; pass options or --show")
	}
	if len(values) > 0 {
		if err := saveMcpConfigValues(values); err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Saved %d recorder option(s).\n", len(values))
	}
	if mcpConfigShow || len(values) > 0 {
		settings, err := loadMcpSettings()
		if err != nil {
			return err
		}
		encoder := json.NewEncoder(cmd.OutOrStdout())
		encoder.SetIndent("", "  ")
		return encoder.Encode(map[string]interface{}{
			"data_dir":         settings.DataDir,
			"memdb_bin":        settings.MemdbBin,
			"memdb_db":         settings.MemdbDB,
			"treechat_mode":    settings.TreechatMode,
			"treechat_team_id": settings.TreechatTeamID,
			"treechat_env":     settings.TreechatEnv,
			"treechat_account": settings.TreechatAccount,
			"recall_on_start":  settings.RecallOnStart,
			"recall_on_prompt": settings.RecallOnPrompt,
		})
	}
	return nil
}

// normalizeTeamTarget accepts a team UUID or a stream link ending in the id.
func normalizeTeamTarget(value string) (string, error) {
	trimmed := strings.TrimSpace(value)
	if looksLikeUUID(trimmed) {
		return trimmed, nil
	}
	parts := strings.Split(strings.TrimRight(trimmed, "/"), "/")
	for index := len(parts) - 1; index >= 0; index-- {
		candidate := strings.SplitN(parts[index], "?", 2)[0]
		if looksLikeUUID(candidate) {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("--treechat-team must be a stream (team) UUID or a link containing one")
}

func parseSinceFlag(value string) (time.Time, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return time.Time{}, nil
	}
	if parsed, err := time.Parse("2006-01-02", trimmed); err == nil {
		return parsed, nil
	}
	if strings.HasSuffix(trimmed, "d") {
		days, err := strconv.Atoi(strings.TrimSuffix(trimmed, "d"))
		if err == nil {
			return time.Now().Add(-time.Duration(days) * 24 * time.Hour), nil
		}
	}
	duration, err := time.ParseDuration(trimmed)
	if err != nil {
		return time.Time{}, fmt.Errorf("--since must be a duration like 30d or 12h, or a date like 2026-09-01")
	}
	return time.Now().Add(-duration), nil
}

func runMcpBackfill(cmd *cobra.Command, args []string) error {
	agents := []recorder.Agent{}
	if mcpBackfillClaude || mcpBackfillAll {
		agents = append(agents, recorder.AgentClaudeCode)
	}
	if mcpBackfillCodex || mcpBackfillAll {
		agents = append(agents, recorder.AgentCodex)
	}
	if len(agents) == 0 {
		return errors.New("choose --claude, --codex, or --all")
	}
	since, err := parseSinceFlag(mcpBackfillSince)
	if err != nil {
		return err
	}
	settings, err := loadMcpSettings()
	if err != nil {
		return err
	}
	rec, err := buildRecorder(settings, !mcpBackfillDryRun)
	if err != nil {
		return err
	}
	rec.Poster = nil // backfills never post
	out := cmd.OutOrStdout()
	total, ingested, skipped, failed := 0, 0, 0, 0
	ctx := context.Background()
	for _, agent := range agents {
		sources, err := recorder.DiscoverTranscripts(agent, since)
		if err != nil {
			return err
		}
		fmt.Fprintf(out, "%s: %d transcript(s)\n", recorder.AgentLabel(agent), len(sources))
		for _, source := range sources {
			if mcpBackfillLimit > 0 && total >= mcpBackfillLimit {
				break
			}
			total++
			session, err := recorder.ParseTranscript(agent, source.Path)
			if err != nil {
				failed++
				fmt.Fprintf(out, "  ! %s: %v\n", source.Path, err)
				continue
			}
			if session.SessionID == "" || len(session.Messages) == 0 {
				skipped++
				continue
			}
			if mcpBackfillDryRun {
				fmt.Fprintf(out, "  %s %s (%d messages)\n", session.SessionID, session.CWD, len(session.Messages))
				continue
			}
			result, err := rec.SyncSession(ctx, session, true)
			if err != nil {
				failed++
				fmt.Fprintf(out, "  ! %s: %v\n", source.Path, err)
				continue
			}
			ingested++
			if result.Ingest.MessagesAdded > 0 || result.Ingest.EventsAdded > 0 {
				fmt.Fprintf(out, "  + %s %s (+%d messages, +%d events)\n", session.SessionID, session.CWD, result.Ingest.MessagesAdded, result.Ingest.EventsAdded)
			}
		}
	}
	fmt.Fprintf(out, "Done: %d transcript(s) seen, %d ingested, %d skipped (empty), %d failed.\n", total, ingested, skipped, failed)
	return nil
}

func runMcpRecord(cmd *cobra.Command, args []string) error {
	agent, err := resolveMcpAgent(mcpAgentFlag)
	if err != nil {
		return err
	}
	settings, err := loadMcpSettings()
	if err != nil {
		return err
	}
	rec, err := buildRecorder(settings, true)
	if err != nil {
		return err
	}
	if !mcpRecordPost {
		rec.Poster = nil
	}
	result, err := rec.SyncTranscript(context.Background(), agent, args[0], "", "", mcpRecordEnded)
	if err != nil {
		return err
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Recorded %s session %s: %d messages, %d turns -> %s (+%d messages, +%d events in memdb)\n",
		recorder.AgentLabel(agent), result.Session.SessionID, len(result.Session.Messages), len(result.Session.Turns()), result.RecordingPath, result.Ingest.MessagesAdded, result.Ingest.EventsAdded)
	if result.TreechatErr != nil {
		fmt.Fprintf(cmd.OutOrStdout(), "Treechat: %v\n", result.TreechatErr)
	} else if result.Treechat.QuestID != "" {
		fmt.Fprintf(cmd.OutOrStdout(), "Treechat thread: %s\n", firstNonEmpty(result.Treechat.QuestURL, result.Treechat.QuestID))
	}
	return nil
}
