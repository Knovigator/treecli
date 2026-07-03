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

Important Treechat action model:
- Use plain generation actions like `flux`, `veo3`, or `kling` when you want a new asset from a prompt.
- Use an `animate_*` action when you want to animate an existing image.
- Use an `edit_*` action when you want to edit an existing image.
- If the task is "animate this previous image" or "edit this previous image", do not pick a plain generation action.
- For post-less local media generation, inspect support with `treectl generate actions --direct-only`; use `treectl generate actions --verbose` or `treectl generate describe <action>` for model descriptions, settings, and examples.
- Generate local media with `treectl generate <action> "prompt" --out <file>` and pass settings with `--input key=value`, `--settings '{...}'`, `--duration`, `--instrumental`, or `--reference` as described by the action.

Turn on live shell completion immediately in bash or zsh:

```bash
if [ -n "${ZSH_VERSION:-}" ]; then autoload -U compinit && compinit; source <(treectl completion zsh); elif command -v complete >/dev/null 2>&1; then source <(treectl completion bash); else echo "Current shell does not support bash completion; use zsh or a bash with progcomp."; fi
```

---

{{AGENTS_MD_BLOCK}}
