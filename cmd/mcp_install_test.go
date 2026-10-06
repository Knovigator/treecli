package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knovigator/treecli/recorder"
)

func TestUpsertTomlTableReplacesOnlyTreecliBlock(t *testing.T) {
	existing := "model = \"gpt-6\"\n\n[mcp_servers.other]\ncommand = \"x\"\n\n[mcp_servers.treecli]\ncommand = \"old\"\n\n[mcp_servers.treecli.env]\nA = \"b\"\n\n[projects.\"/tmp/x\"]\ntrust_level = \"trusted\"\n"
	updated := upsertTomlTable(existing, "mcp_servers.treecli", tomlServerBlock("/usr/local/bin/treecli", recorder.AgentCodex))
	if strings.Contains(updated, `command = "old"`) || strings.Contains(updated, `A = "b"`) {
		t.Fatalf("old treecli block (and its sub-table) must be removed:\n%s", updated)
	}
	if !strings.Contains(updated, "[mcp_servers.other]\ncommand = \"x\"") || !strings.Contains(updated, "[projects.\"/tmp/x\"]\ntrust_level = \"trusted\"") {
		t.Fatalf("other tables must survive:\n%s", updated)
	}
	if strings.Count(updated, "[mcp_servers.treecli]") != 1 || !strings.Contains(updated, `args = ["mcp", "serve", "--agent", "codex"]`) || !strings.Contains(updated, `env = { TREECLI_AGENT = "codex" }`) {
		t.Fatalf("new block missing or duplicated:\n%s", updated)
	}
	removed := upsertTomlTable(updated, "mcp_servers.treecli", "")
	if strings.Contains(removed, "treecli") {
		t.Fatalf("removal must drop the block:\n%s", removed)
	}
	if strings.Contains(removed, "\n\n\n") {
		t.Fatalf("removal must not leave triple blank lines:\n%q", removed)
	}
	if got := upsertTomlTable("", "mcp_servers.treecli", tomlServerBlock("/t", recorder.AgentGrok)); !strings.HasPrefix(got, "[mcp_servers.treecli]\n") {
		t.Fatalf("empty file gets just the block:\n%s", got)
	}
}

func TestTomlStringEscapesQuotesAndBackslashes(t *testing.T) {
	if got := tomlString(`C:\Program Files\tree"cli`); got != `"C:\\Program Files\\tree\"cli"` {
		t.Fatalf("unexpected escaping %s", got)
	}
}

func TestMergeHooksFileAddsReplacesAndRemovesTreecliEntries(t *testing.T) {
	existing := []byte(`{"hooks":{"Stop":[{"hooks":[{"type":"command","command":"echo keep"}]},{"hooks":[{"type":"command","command":"/old/treecli mcp hook --agent codex"}]}]},"other":true}`)
	merged, err := mergeHooksFile(existing, "/new/treecli", recorder.AgentCodex, false)
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]interface{}
	if err := json.Unmarshal(merged, &document); err != nil {
		t.Fatalf("merged hooks must be JSON: %v", err)
	}
	if document["other"] != true {
		t.Fatal("unrelated top-level keys must survive")
	}
	hooks := document["hooks"].(map[string]interface{})
	stop := hooks["Stop"].([]interface{})
	if len(stop) != 2 {
		t.Fatalf("Stop should keep the foreign entry and hold one treecli entry, got %d", len(stop))
	}
	if strings.Contains(string(merged), "/old/treecli") || !strings.Contains(string(merged), "/new/treecli mcp hook --agent codex") {
		t.Fatalf("old treecli entry must be replaced:\n%s", merged)
	}
	for _, event := range []string{"SessionStart", "UserPromptSubmit", "SessionEnd"} {
		if _, ok := hooks[event]; !ok {
			t.Fatalf("event %s missing after merge", event)
		}
	}
	if _, ok := hooks["PostToolUse"]; ok {
		t.Fatal("codex does not need PostToolUse hooks (its transcript carries tool calls)")
	}
	removed, err := mergeHooksFile(merged, "/new/treecli", recorder.AgentCodex, true)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(removed), "treecli") || !strings.Contains(string(removed), "echo keep") {
		t.Fatalf("removal must keep foreign hooks only:\n%s", removed)
	}
	onlyOurs, _ := mergeHooksFile(nil, "/t", recorder.AgentGrok, false)
	gone, err := mergeHooksFile(onlyOurs, "/t", recorder.AgentGrok, true)
	if err != nil || gone != nil {
		t.Fatalf("removing the last entries must yield an empty document (nil), got %s (%v)", gone, err)
	}
	if !strings.Contains(string(onlyOurs), `"PostToolUse"`) || !strings.Contains(string(onlyOurs), `"matcher": ".*"`) {
		t.Fatalf("grok hooks must include a PostToolUse catch-all:\n%s", onlyOurs)
	}
}

func TestPlanClaudeInstallWritesAValidPluginDirectory(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", "")
	paths := installPaths{Home: home}
	planned, err := planAgent(paths, recorder.AgentClaudeCode, "/opt/treecli", false)
	if err != nil {
		t.Fatal(err)
	}
	if err := applyPlan(os.Stderr, planned, false); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(home, ".claude", "skills", "treecli")
	for _, relative := range []string{".claude-plugin/plugin.json", ".mcp.json", "hooks/hooks.json", "skills/treecli-memory/SKILL.md"} {
		if _, err := os.Stat(filepath.Join(dir, relative)); err != nil {
			t.Fatalf("%s missing: %v", relative, err)
		}
	}
	var manifest map[string]interface{}
	data, _ := os.ReadFile(filepath.Join(dir, ".claude-plugin", "plugin.json"))
	if err := json.Unmarshal(data, &manifest); err != nil || manifest["name"] != "treecli" {
		t.Fatalf("bad manifest %s (%v)", data, err)
	}
	var servers map[string]map[string]interface{}
	data, _ = os.ReadFile(filepath.Join(dir, ".mcp.json"))
	if err := json.Unmarshal(data, &servers); err != nil || servers["treecli"]["command"] != "/opt/treecli" {
		t.Fatalf("bad .mcp.json %s (%v)", data, err)
	}
	if args, _ := json.Marshal(servers["treecli"]["args"]); string(args) != `["mcp","serve","--agent","claude-code"]` {
		t.Fatalf("unexpected server args %s", args)
	}
	data, _ = os.ReadFile(filepath.Join(dir, "hooks", "hooks.json"))
	if !strings.Contains(string(data), "/opt/treecli mcp hook --agent claude-code") {
		t.Fatalf("hooks must call the installed binary:\n%s", data)
	}
	status := agentStatus{}
	_, mcpErr := os.Stat(filepath.Join(dir, ".mcp.json"))
	status.MCP = mcpErr == nil
	if !status.MCP {
		t.Fatal("status must detect the plugin")
	}
	removal, err := planAgent(paths, recorder.AgentClaudeCode, "/opt/treecli", true)
	if err != nil || len(removal) != 1 || !removal[0].Remove || removal[0].Path != dir {
		t.Fatalf("uninstall must remove the plugin dir, got %+v (%v)", removal, err)
	}
}

func TestPlanTomlAgentInstallIsIdempotent(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CODEX_HOME", "")
	t.Setenv("GROK_HOME", "")
	paths := installPaths{Home: home}
	first, err := planAgent(paths, recorder.AgentGrok, "/opt/treecli", false)
	if err != nil {
		t.Fatal(err)
	}
	if len(first) != 2 {
		t.Fatalf("grok install writes config + hooks, got %+v", first)
	}
	if err := applyPlan(os.Stderr, first, false); err != nil {
		t.Fatal(err)
	}
	if !tomlHasTable(paths.grokConfig(), "mcp_servers.treecli") {
		t.Fatal("grok config missing the server table")
	}
	second, err := planAgent(paths, recorder.AgentGrok, "/opt/treecli", false)
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range second {
		if strings.HasSuffix(file.Path, "config.toml") {
			t.Fatalf("unchanged config must not be rewritten: %+v", second)
		}
	}
	removal, err := planAgent(paths, recorder.AgentGrok, "/opt/treecli", true)
	if err != nil || len(removal) != 2 {
		t.Fatalf("uninstall must rewrite config and remove hooks, got %+v (%v)", removal, err)
	}
}

func TestShellQuote(t *testing.T) {
	if shellQuote("/usr/local/bin/treecli") != "/usr/local/bin/treecli" {
		t.Fatal("plain paths stay bare")
	}
	if got := shellQuote("/Users/Me/My Apps/tree'cli"); got != `'/Users/Me/My Apps/tree'\''cli'` {
		t.Fatalf("unexpected quoting %s", got)
	}
}
