---
name: treechat-ai
description: Generate images, video, lip sync, music, voices and 4K upscales with Treechat AI, billed to the user's Treechat account within the spending caps they set. Use when the user asks to generate, render, animate, lip-sync, upscale or make music with a model, or names a Treechat AI model.
---

# Treechat AI

The `treechat` MCP server (shipped with this plugin) runs Treechat AI's models on the user's own
Treechat account. Every paid run is charged to that account at normal Treechat pricing, inside
two caps the user chose when they connected: one per run, one per rolling 24 hours.

## First use

1. If the `treechat` tools ask for sign-in, tell the user to run `/mcp`, pick **treechat**, and
   authenticate in the browser. They sign in to Treechat, approve "Treechat AI", and set their caps.
2. Call `enable_generation`. It is free and checks that generation is ready. If it says billing
   isn't set up, give the user the link it returns. Never pick or raise caps for them.

## Running a model

1. `list_generation_actions` shows the models, their settings and reference slots. Pick the one
   that fits; say which and why in one line.
2. References (a still to animate, a voice to lip-sync): a public URL works as `reference`, or
   upload a local file with `register_upload`, `upload_file`, `finish_reference_upload` and pass
   the returned `run:<id>`.
3. **Quote first.** `quote_generation` with the model, prompt, settings and references. Show the
   user the prompt, the references and the quoted price, and wait for an explicit yes. A yes
   covers that one run; a retry with the same inputs reuses the same quote.
   - Exception: if the user has said runs within their saved caps don't need asking, use
     `generate_media` directly.
4. `create_generation` with the `quote_id` and an `idempotency_key` you choose. Reuse both on
   every retry, so a retry can never charge twice.
5. If the result is `approval_required`, the run would go over a cap. Give the user the
   approval link and the quoted cost. Only they can approve it in their browser; a "yes" in
   chat does not.
6. Poll `get_generation` until it finishes, then download the media and tell the user where
   it is and what it cost.

`get_generation_budget` shows what is left of today's cap.

## Rules

- Never raise, reset or work around a spending cap, and never reconnect with higher limits
  unless the user asks.
- Never start a second paid run because the first one's outcome is unclear. Check it with
  `get_generation` first.
- Prompts and references go to Treechat and its model providers. Don't send anything the user
  marked private.
- Never post to Treechat with these tools. They make media; publishing is the user's call.
