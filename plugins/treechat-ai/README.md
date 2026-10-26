# Treechat AI for Claude Code

Image, video, lip-sync, music, voice and 4K-upscale models from [Treechat](https://treechat.com), run by Claude Code on your own Treechat account.

## Install

```
/plugin marketplace add Knovigator/treecli
/plugin install treechat-ai@treechat
```

Then run `/mcp`, pick **treechat** and authenticate. You sign in to Treechat, approve "Treechat AI", and set two spending caps: one per run and one per 24 hours (defaults $10 and $100). Runs are billed to your Treechat account at normal prices.

## Use

Ask for what you want ("animate this still for 5 seconds", "lip-sync this vocal to her face", "upscale this clip to 4K"), or use:

| Command | Does |
| --- | --- |
| `/treechat-ai:models` | the models you can run |
| `/treechat-ai:generate <what>` | quote one run, then run it after your yes |
| `/treechat-ai:budget` | what's left of your caps |

Every run is quoted first, and Claude waits for your yes. A run over your caps waits for your approval in the browser; Claude can't approve it for you.
