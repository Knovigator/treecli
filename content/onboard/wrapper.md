# treectl Onboard

This repo ships agent-facing `treectl` guidance and packaged skills inside the CLI binary.

## Use This Output

Add treectl guidance to your instruction file:
- `AGENTS.md` for Codex and generic agent tools
- `CLAUDE.md` for Claude Code

Append if the file already exists.

Raw block for direct append:

```bash
# Full agents.md block (default long)
treectl onboard --agents-md >> AGENTS.md

# Explicit short/long variants
treectl onboard --agents-md --short >> AGENTS.md
treectl onboard --agents-md --long >> AGENTS.md
```

Install packaged treectl skills instead of copying them by hand:

```bash
treectl skills list
treectl skills install treectl-basic-usage --codex
treectl skills install treectl-action-workflows --claude
treectl skills install all --pi
```

Turn on live shell completion immediately in bash or zsh:

```bash
source <(treectl completion $(basename "$SHELL"))
```

---

{{AGENTS_MD_BLOCK}}
