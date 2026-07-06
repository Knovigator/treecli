# treecli Onboard

This repo ships agent-facing `treecli` guidance and packaged skills inside the CLI binary.

## Use This Output

Add treecli guidance to your instruction file:
- `AGENTS.md` for Codex and generic agent tools
- `CLAUDE.md` for Claude Code

Append if the file already exists.

Raw block for direct append:

```bash
# Full agents.md block (default long)
treecli onboard --agents-md >> AGENTS.md

# Explicit short/long variants
treecli onboard --agents-md --short >> AGENTS.md
treecli onboard --agents-md --long >> AGENTS.md
```

Install packaged treecli skills instead of copying them by hand:

```bash
treecli skills list
treecli skills install treecli-basic-usage --codex
treecli skills install treecli-action-workflows --claude
treecli skills install all --pi
```

Important Treechat action model:
- Use plain generation actions like `flux`, `veo3`, or `kling` when you want a new asset from a prompt.
- Use an `animate_*` action when you want to animate an existing image.
- Use an `edit_*` action when you want to edit an existing image.
- Use `eleven_tts` for text-to-speech and `video_sfx` for video sound effects; CLI aliases include `eleven`, `elevenlabs`, `11`, `sfx`, `mmaudio`, and `foley`.
- If the task is "animate this previous image" or "edit this previous image", do not pick a plain generation action.
- Use `--payment usd` for Stripe metered AI billing or `--payment bsv` / `--payment bitcoinsv` for Bitcoin SV; omit it to use the account default.
- For post-less local media generation, inspect support with `treecli generate actions --direct-only`; use `treecli generate actions --verbose` or `treecli generate describe <action>` for model descriptions, inputs, settings, reference behavior, and examples.
- Generate local media with `treecli generate <action> "prompt" --out <file>` and pass settings with `--input key=value`, `--settings '{...}'`, `--duration`, `--instrumental`, `--reference`, or `--payment` as described by the action and billing intent.
- Check for a newer CLI release with `treecli update --check`; install it with `treecli update`. Use `--json` for machine-readable update status.

Turn on live shell completion immediately in bash or zsh:

```bash
if [ -n "${ZSH_VERSION:-}" ]; then autoload -U compinit && compinit; source <(treecli completion zsh); elif command -v complete >/dev/null 2>&1; then source <(treecli completion bash); else echo "Current shell does not support bash completion; use zsh or a bash with progcomp."; fi
```

---

{{AGENTS_MD_BLOCK}}
