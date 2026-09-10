package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	treeclicontent "github.com/Knovigator/treecli/content"
	"github.com/Knovigator/treecli/recorder"
	"github.com/spf13/cobra"
)

var mcpInstallCmd = &cobra.Command{
	Use:   "install",
	Short: "Register the treecli MCP server and recording hooks with Claude Code, Codex and/or Grok",
	Long: `Writes the per-agent plugin/config so the agent launches "treecli mcp serve" as an
MCP server and runs "treecli mcp hook" on session lifecycle events.

  --claude   plugin directory at ~/.claude/skills/treecli (auto-loads next session)
  --codex    [mcp_servers.treecli] in ~/.codex/config.toml + entries in ~/.codex/hooks.json
  --grok     [mcp_servers.treecli] in ~/.grok/config.toml + ~/.grok/hooks/treecli.json (Grok Build);
             the Grok Bot desktop app needs the server added in its MCP settings (printed)

Re-running is safe: existing treecli entries are replaced, nothing else is touched.`,
	Args: cobra.NoArgs,
	RunE: runMcpInstall,
}

var mcpUninstallCmd = &cobra.Command{
	Use:   "uninstall",
	Short: "Remove the treecli MCP server and hooks from Claude Code, Codex and/or Grok",
	Args:  cobra.NoArgs,
	RunE:  runMcpUninstall,
}

var (
	mcpInstallClaude   bool
	mcpInstallCodex    bool
	mcpInstallGrok     bool
	mcpInstallAll      bool
	mcpInstallBinary   string
	mcpInstallTeam     string
	mcpInstallMode     string
	mcpInstallMemdbBin string
	mcpInstallDryRun   bool
	mcpInstallNoRecall bool
)

func init() {
	for _, command := range []*cobra.Command{mcpInstallCmd, mcpUninstallCmd} {
		command.Flags().BoolVar(&mcpInstallClaude, "claude", false, "Claude Code")
		command.Flags().BoolVar(&mcpInstallCodex, "codex", false, "Codex CLI / desktop")
		command.Flags().BoolVar(&mcpInstallGrok, "grok", false, "Grok Build CLI (and instructions for Grok Bot)")
		command.Flags().BoolVar(&mcpInstallAll, "all", false, "Every supported agent")
		command.Flags().BoolVar(&mcpInstallDryRun, "dry-run", false, "Print the files that would change without writing")
	}
	mcpInstallCmd.Flags().StringVar(&mcpInstallBinary, "binary", "", "treecli executable the agents should launch (default: this binary)")
	mcpInstallCmd.Flags().StringVar(&mcpInstallTeam, "treechat-team", "", "Also configure the Treechat stream (team) id or link to record into")
	mcpInstallCmd.Flags().StringVar(&mcpInstallMode, "treechat-mode", "", "Also configure Treechat recording: off, session, or turns")
	mcpInstallCmd.Flags().StringVar(&mcpInstallMemdbBin, "memdb-bin", "", "Also configure the memdb-rust binary path")
	mcpInstallCmd.Flags().BoolVar(&mcpInstallNoRecall, "no-recall", false, "Do not inject memdb context at session start / prompts")
}

func selectedInstallAgents() ([]recorder.Agent, error) {
	agents := []recorder.Agent{}
	if mcpInstallClaude || mcpInstallAll {
		agents = append(agents, recorder.AgentClaudeCode)
	}
	if mcpInstallCodex || mcpInstallAll {
		agents = append(agents, recorder.AgentCodex)
	}
	if mcpInstallGrok || mcpInstallAll {
		agents = append(agents, recorder.AgentGrok)
	}
	if len(agents) == 0 {
		return nil, errors.New("choose at least one of --claude, --codex, --grok, or --all")
	}
	return agents, nil
}

// installPaths centralizes where each agent keeps its configuration so tests
// can point them at a temporary home.
type installPaths struct {
	Home string
}

func defaultInstallPaths() installPaths {
	return installPaths{Home: userHomeDir()}
}

func (p installPaths) claudeHome() string {
	if value := strings.TrimSpace(os.Getenv("CLAUDE_CONFIG_DIR")); value != "" {
		return value
	}
	return filepath.Join(p.Home, ".claude")
}

func (p installPaths) codexHome() string {
	if value := strings.TrimSpace(os.Getenv("CODEX_HOME")); value != "" {
		return value
	}
	return filepath.Join(p.Home, ".codex")
}

func (p installPaths) grokHome() string {
	if value := strings.TrimSpace(os.Getenv("GROK_HOME")); value != "" {
		return value
	}
	return filepath.Join(p.Home, ".grok")
}

func (p installPaths) claudePluginDir() string {
	return filepath.Join(p.claudeHome(), "skills", "treecli")
}
func (p installPaths) codexConfig() string { return filepath.Join(p.codexHome(), "config.toml") }
func (p installPaths) codexHooks() string  { return filepath.Join(p.codexHome(), "hooks.json") }
func (p installPaths) grokConfig() string  { return filepath.Join(p.grokHome(), "config.toml") }
func (p installPaths) grokHooks() string   { return filepath.Join(p.grokHome(), "hooks", "treecli.json") }

type agentStatus struct {
	MCP    bool   `json:"mcp"`
	Hooks  bool   `json:"hooks"`
	Detail string `json:"detail,omitempty"`
}

func inspectAgentInstall(agent recorder.Agent) agentStatus {
	paths := defaultInstallPaths()
	switch agent {
	case recorder.AgentClaudeCode:
		dir := paths.claudePluginDir()
		_, mcpErr := os.Stat(filepath.Join(dir, ".mcp.json"))
		_, hooksErr := os.Stat(filepath.Join(dir, "hooks", "hooks.json"))
		return agentStatus{MCP: mcpErr == nil, Hooks: hooksErr == nil, Detail: dir}
	case recorder.AgentCodex:
		return agentStatus{MCP: tomlHasTable(paths.codexConfig(), "mcp_servers.treecli"), Hooks: hooksFileHasTreecli(paths.codexHooks()), Detail: paths.codexHome()}
	case recorder.AgentGrok:
		_, hooksErr := os.Stat(paths.grokHooks())
		return agentStatus{MCP: tomlHasTable(paths.grokConfig(), "mcp_servers.treecli"), Hooks: hooksErr == nil, Detail: paths.grokHome()}
	}
	return agentStatus{}
}

// hookSpec describes one hook registration in the shared Claude-style dialect.
type hookSpec struct {
	Event   string
	Matcher string
	Timeout int
}

func hookSpecsFor(agent recorder.Agent) []hookSpec {
	specs := []hookSpec{
		{Event: recorder.EventSessionStart, Timeout: 20},
		{Event: recorder.EventUserPromptSubmit, Timeout: 15},
		{Event: recorder.EventStop, Timeout: 60},
		{Event: recorder.EventSessionEnd, Timeout: 60},
	}
	if agent == recorder.AgentGrok {
		// Grok keeps no transcript treecli can read; tool calls arrive through hooks.
		specs = append(specs, hookSpec{Event: recorder.EventPostToolUse, Matcher: ".*", Timeout: 20})
	}
	return specs
}

func hookCommand(binary string, agent recorder.Agent) string {
	return fmt.Sprintf("%s mcp hook --agent %s", shellQuote(binary), agent)
}

func shellQuote(value string) string {
	if value == "" {
		return "''"
	}
	if regexp.MustCompile(`^[A-Za-z0-9_./~+=:@%-]+$`).MatchString(value) {
		return value
	}
	return "'" + strings.ReplaceAll(value, "'", `'\''`) + "'"
}

// hooksDocument renders {"hooks": {Event: [{matcher?, hooks: [{type, command, timeout}]}]}}.
func hooksDocument(binary string, agent recorder.Agent) map[string]interface{} {
	events := map[string]interface{}{}
	for _, spec := range hookSpecsFor(agent) {
		entry := map[string]interface{}{
			"hooks": []interface{}{map[string]interface{}{
				"type":    "command",
				"command": hookCommand(binary, agent),
				"timeout": spec.Timeout,
			}},
		}
		if spec.Matcher != "" {
			entry["matcher"] = spec.Matcher
		}
		events[spec.Event] = []interface{}{entry}
	}
	return map[string]interface{}{"hooks": events}
}

type plannedFile struct {
	Path    string
	Content string
	Remove  bool
}

func pluginVersion() string {
	version := strings.TrimPrefix(strings.TrimSpace(CurrentVersion), "v")
	if regexp.MustCompile(`^\d+\.\d+\.\d+`).MatchString(version) {
		return version
	}
	return "0.0.0-dev"
}

func mcpServerEntry(binary string, agent recorder.Agent) map[string]interface{} {
	return map[string]interface{}{
		"command": binary,
		"args":    []string{"mcp", "serve", "--agent", string(agent)},
		"env":     map[string]string{"TREECLI_AGENT": string(agent)},
	}
}

func planClaudeInstall(paths installPaths, binary string) ([]plannedFile, error) {
	dir := paths.claudePluginDir()
	manifest, _ := json.MarshalIndent(map[string]interface{}{
		"name":        "treecli",
		"description": "Records this Claude Code session into memdb (durable local memory) and Treechat, and serves memory_recall / memory_search / memory_learn tools over MCP.",
		"version":     pluginVersion(),
		"author":      map[string]string{"name": "Knovigator", "url": "https://github.com/Knovigator/treecli"},
	}, "", "  ")
	mcpJSON, _ := json.MarshalIndent(map[string]interface{}{"treecli": mcpServerEntry(binary, recorder.AgentClaudeCode)}, "", "  ")
	hooksJSON, _ := json.MarshalIndent(hooksDocument(binary, recorder.AgentClaudeCode), "", "  ")
	skill, err := treeclicontent.GetPackagedSkill("treecli-memory")
	if err != nil {
		return nil, err
	}
	if skill == nil {
		return nil, errors.New("packaged skill treecli-memory is missing")
	}
	return []plannedFile{
		{Path: filepath.Join(dir, ".claude-plugin", "plugin.json"), Content: string(manifest) + "\n"},
		{Path: filepath.Join(dir, ".mcp.json"), Content: string(mcpJSON) + "\n"},
		{Path: filepath.Join(dir, "hooks", "hooks.json"), Content: string(hooksJSON) + "\n"},
		{Path: filepath.Join(dir, "skills", "treecli-memory", "SKILL.md"), Content: skill.SkillMD},
	}, nil
}

// tomlServerBlock renders the [mcp_servers.treecli] table Codex and Grok Build read.
func tomlServerBlock(binary string, agent recorder.Agent) string {
	return strings.Join([]string{
		"[mcp_servers.treecli]",
		fmt.Sprintf("command = %s", tomlString(binary)),
		fmt.Sprintf(`args = ["mcp", "serve", "--agent", "%s"]`, agent),
		fmt.Sprintf(`env = { TREECLI_AGENT = "%s" }`, agent),
		"",
	}, "\n")
}

func tomlString(value string) string {
	encoded, _ := json.Marshal(value) // JSON string escaping is valid TOML basic-string escaping
	return string(encoded)
}

// upsertTomlTable replaces or appends a top-level table (and its sub-tables) in a TOML file.
func upsertTomlTable(existing, table, block string) string {
	stripped := removeTomlTable(existing, table)
	if strings.TrimSpace(block) == "" {
		return stripped
	}
	if strings.TrimSpace(stripped) == "" {
		return block
	}
	return strings.TrimRight(stripped, "\n") + "\n\n" + block
}

func removeTomlTable(existing, table string) string {
	lines := strings.Split(existing, "\n")
	kept := make([]string, 0, len(lines))
	skipping := false
	header := regexp.MustCompile(`^\s*\[\[?([^\]]+)\]\]?\s*(#.*)?$`)
	for _, line := range lines {
		if match := header.FindStringSubmatch(line); match != nil {
			name := strings.TrimSpace(match[1])
			skipping = name == table || strings.HasPrefix(name, table+".")
		}
		if skipping {
			continue
		}
		kept = append(kept, line)
	}
	result := strings.Join(kept, "\n")
	for strings.Contains(result, "\n\n\n") {
		result = strings.ReplaceAll(result, "\n\n\n", "\n\n")
	}
	return result
}

func tomlHasTable(path, table string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	return regexp.MustCompile(`(?m)^\s*\[` + regexp.QuoteMeta(table) + `\]\s*$`).Match(data)
}

// mergeHooksFile inserts treecli's hook entries into a shared hooks.json,
// replacing any earlier treecli entries and leaving other hooks alone.
func mergeHooksFile(existing []byte, binary string, agent recorder.Agent, remove bool) ([]byte, error) {
	document := map[string]interface{}{}
	if len(strings.TrimSpace(string(existing))) > 0 {
		if err := json.Unmarshal(existing, &document); err != nil {
			return nil, fmt.Errorf("existing hooks file is not valid JSON: %w", err)
		}
	}
	hooks, _ := document["hooks"].(map[string]interface{})
	if hooks == nil {
		hooks = map[string]interface{}{}
	}
	ours := hooksDocument(binary, agent)["hooks"].(map[string]interface{})
	events := map[string]bool{}
	for event := range hooks {
		events[event] = true
	}
	for event := range ours {
		events[event] = true
	}
	for event := range events {
		entries, _ := hooks[event].([]interface{})
		filtered := make([]interface{}, 0, len(entries))
		for _, entry := range entries {
			if !hookEntryIsTreecli(entry) {
				filtered = append(filtered, entry)
			}
		}
		if !remove {
			if mine, ok := ours[event].([]interface{}); ok {
				filtered = append(filtered, mine...)
			}
		}
		if len(filtered) == 0 {
			delete(hooks, event)
		} else {
			hooks[event] = filtered
		}
	}
	if len(hooks) == 0 {
		delete(document, "hooks")
	} else {
		document["hooks"] = hooks
	}
	if len(document) == 0 {
		return nil, nil
	}
	encoded, err := json.MarshalIndent(document, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(encoded, '\n'), nil
}

func hookEntryIsTreecli(entry interface{}) bool {
	object, ok := entry.(map[string]interface{})
	if !ok {
		return false
	}
	handlers, _ := object["hooks"].([]interface{})
	for _, handler := range handlers {
		if handlerObject, ok := handler.(map[string]interface{}); ok {
			command, _ := handlerObject["command"].(string)
			// Match the invocation shape, not the binary name: the installed
			// treecli may live at any path.
			if strings.Contains(command, " mcp hook --agent ") {
				return true
			}
		}
	}
	return false
}

func hooksFileHasTreecli(path string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	return strings.Contains(string(data), " mcp hook --agent ")
}

func planTomlAgentInstall(configPath, hooksPath string, mergeHooks bool, binary string, agent recorder.Agent, remove bool) ([]plannedFile, error) {
	planned := []plannedFile{}
	existingConfig, err := os.ReadFile(configPath)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	block := ""
	if !remove {
		block = tomlServerBlock(binary, agent)
	}
	updatedConfig := upsertTomlTable(string(existingConfig), "mcp_servers.treecli", block)
	if updatedConfig != string(existingConfig) {
		planned = append(planned, plannedFile{Path: configPath, Content: updatedConfig})
	}
	if mergeHooks {
		existingHooks, err := os.ReadFile(hooksPath)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
		merged, err := mergeHooksFile(existingHooks, binary, agent, remove)
		if err != nil {
			return nil, err
		}
		if merged == nil {
			if len(existingHooks) > 0 {
				planned = append(planned, plannedFile{Path: hooksPath, Remove: true})
			}
		} else if string(merged) != string(existingHooks) {
			planned = append(planned, plannedFile{Path: hooksPath, Content: string(merged)})
		}
	} else if remove {
		if _, err := os.Stat(hooksPath); err == nil {
			planned = append(planned, plannedFile{Path: hooksPath, Remove: true})
		}
	} else {
		encoded, _ := json.MarshalIndent(hooksDocument(binary, agent), "", "  ")
		planned = append(planned, plannedFile{Path: hooksPath, Content: string(encoded) + "\n"})
	}
	return planned, nil
}

func planAgent(paths installPaths, agent recorder.Agent, binary string, remove bool) ([]plannedFile, error) {
	switch agent {
	case recorder.AgentClaudeCode:
		if remove {
			if _, err := os.Stat(paths.claudePluginDir()); err == nil {
				return []plannedFile{{Path: paths.claudePluginDir(), Remove: true}}, nil
			}
			return nil, nil
		}
		return planClaudeInstall(paths, binary)
	case recorder.AgentCodex:
		return planTomlAgentInstall(paths.codexConfig(), paths.codexHooks(), true, binary, agent, remove)
	case recorder.AgentGrok:
		return planTomlAgentInstall(paths.grokConfig(), paths.grokHooks(), false, binary, agent, remove)
	}
	return nil, fmt.Errorf("unsupported agent %q", agent)
}

func applyPlan(out io.Writer, planned []plannedFile, dryRun bool) error {
	for _, file := range planned {
		if file.Remove {
			if dryRun {
				fmt.Fprintf(out, "  would remove %s\n", file.Path)
				continue
			}
			if err := os.RemoveAll(file.Path); err != nil {
				return err
			}
			fmt.Fprintf(out, "  removed %s\n", file.Path)
			continue
		}
		if dryRun {
			fmt.Fprintf(out, "  would write %s\n", file.Path)
			continue
		}
		if err := os.MkdirAll(filepath.Dir(file.Path), 0o700); err != nil {
			return err
		}
		mode := os.FileMode(0o644)
		if strings.HasSuffix(file.Path, ".toml") || strings.HasSuffix(file.Path, "hooks.json") {
			mode = 0o600
		}
		if err := os.WriteFile(file.Path, []byte(file.Content), mode); err != nil {
			return err
		}
		fmt.Fprintf(out, "  wrote %s\n", file.Path)
	}
	return nil
}

func resolveInstallBinary(flag string) (string, error) {
	if strings.TrimSpace(flag) != "" {
		absolute, err := filepath.Abs(strings.TrimSpace(flag))
		if err != nil {
			return "", err
		}
		return absolute, nil
	}
	executable, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("locating this treecli binary: %w", err)
	}
	if resolved, err := filepath.EvalSymlinks(executable); err == nil {
		executable = resolved
	}
	return executable, nil
}

func grokBotInstructions(binary string) string {
	snippet, _ := json.MarshalIndent(map[string]interface{}{"mcpServers": map[string]interface{}{"treecli": mcpServerEntry(binary, recorder.AgentGrok)}}, "", "  ")
	return "Grok Bot (desktop app) keeps its MCP servers in its own settings UI. Add a stdio server named treecli with:\n" +
		"  command: " + binary + "\n" +
		"  args:    mcp serve --agent grok\n" +
		"  env:     TREECLI_AGENT=grok\n" +
		"or paste this where it accepts JSON:\n" + string(snippet) + "\n" +
		"Grok Bot has no hooks; only what the agent records through the session_note / memory_learn tools is captured."
}

func runMcpInstall(cmd *cobra.Command, args []string) error {
	agents, err := selectedInstallAgents()
	if err != nil {
		return err
	}
	binary, err := resolveInstallBinary(mcpInstallBinary)
	if err != nil {
		return err
	}
	out := cmd.OutOrStdout()
	configValues := map[string]interface{}{}
	if mcpInstallTeam != "" {
		teamID, err := normalizeTeamTarget(mcpInstallTeam)
		if err != nil {
			return err
		}
		configValues[mcpConfigTreechatTeam] = teamID
	}
	if mcpInstallMode != "" {
		mode, err := recorder.ParseTreechatMode(mcpInstallMode)
		if err != nil {
			return err
		}
		configValues[mcpConfigTreechatMode] = mode
	}
	if mcpInstallMemdbBin != "" {
		if _, err := recorder.ResolveMemdbBinary(mcpInstallMemdbBin); err != nil {
			return fmt.Errorf("--memdb-bin %s is not an executable file", mcpInstallMemdbBin)
		}
		configValues[mcpConfigMemdbBin] = mcpInstallMemdbBin
	}
	if mcpInstallNoRecall {
		configValues[mcpConfigRecallOnStart] = false
		configValues[mcpConfigRecallOnPrompt] = false
	}
	if len(configValues) > 0 && !mcpInstallDryRun {
		if err := saveMcpConfigValues(configValues); err != nil {
			return err
		}
	}
	settings, err := loadMcpSettings()
	if err != nil {
		return err
	}
	if settings.MemdbBin == "" {
		fmt.Fprintf(out, "Warning: %v\n         Recording will wait until memdb-rust is available; hooks and the MCP server are installed regardless.\n", settings.MemdbBinErr)
	} else if !mcpInstallDryRun {
		memdb := &recorder.Memdb{Binary: settings.MemdbBin, DBPath: settings.MemdbDB}
		if err := memdb.Init(cmd.Context()); err != nil {
			fmt.Fprintf(out, "Warning: memdb init failed: %v\n", err)
		} else {
			fmt.Fprintf(out, "memdb: %s (db %s)\n", settings.MemdbBin, settings.MemdbDB)
		}
	}
	paths := defaultInstallPaths()
	for _, agent := range agents {
		fmt.Fprintf(out, "%s:\n", recorder.AgentLabel(agent))
		planned, err := planAgent(paths, agent, binary, false)
		if err != nil {
			return err
		}
		if len(planned) == 0 {
			fmt.Fprintln(out, "  already up to date")
		}
		if err := applyPlan(out, planned, mcpInstallDryRun); err != nil {
			return err
		}
		switch agent {
		case recorder.AgentClaudeCode:
			fmt.Fprintln(out, "  Loads as treecli@skills-dir on the next Claude Code session (check with `claude plugin list`).")
		case recorder.AgentCodex:
			fmt.Fprintln(out, "  Codex asks you to trust new hooks once: run /hooks inside Codex (or `codex exec --dangerously-bypass-hook-trust` for automation).")
		case recorder.AgentGrok:
			fmt.Fprintln(out, "  Grok Build reads ~/.grok/config.toml and ~/.grok/hooks/*.json on its next start.")
			fmt.Fprintln(out, "  "+strings.ReplaceAll(grokBotInstructions(binary), "\n", "\n  "))
		}
	}
	fmt.Fprintf(out, "Treechat recording: mode=%s team=%s\n", settings.TreechatMode, firstNonEmpty(settings.TreechatTeamID, "(private threads)"))
	if settings.TreechatMode == recorder.TreechatModeOff {
		fmt.Fprintln(out, "  Sessions stay local. To mirror them into a Treechat stream: treecli mcp config --treechat-mode session --treechat-team <stream id or link>")
	}
	return nil
}

func runMcpUninstall(cmd *cobra.Command, args []string) error {
	agents, err := selectedInstallAgents()
	if err != nil {
		return err
	}
	out := cmd.OutOrStdout()
	paths := defaultInstallPaths()
	for _, agent := range agents {
		fmt.Fprintf(out, "%s:\n", recorder.AgentLabel(agent))
		planned, err := planAgent(paths, agent, "treecli", true)
		if err != nil {
			return err
		}
		if len(planned) == 0 {
			fmt.Fprintln(out, "  nothing installed")
		}
		if err := applyPlan(out, planned, mcpInstallDryRun); err != nil {
			return err
		}
	}
	return nil
}
