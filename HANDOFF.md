# `treecli billing` — handoff to Codex

Adds a two-lane `billing` command. Ships the **setup halves** of both lanes; the
on-chain BSV **spend/settlement** is intentionally left as a seam for Codex
(that's where the JS crypto / bsv service lives). Build, vet, and tests are green.

Complements the existing `--payment usd|bsv` flag on `generate` (per-run mode via
`normalizePaymentMode`): `billing mode` sets the account's *default* mode; the
flag overrides per generation.

## The model (agreed in design)

| Lane | Auth | Pays via | Kind |
| --- | --- | --- | --- |
| **USD** | requires `treecli login` | Stripe card on file | pay-as-you-go, post-paid, no balance |
| **BSV** | **no login** | a local treechat-style wallet | pre-paid, spend down a balance, auto-pay default |

The wallet *is* the payment credential, so the BSV lane needs no account. USD is
account-tied (Stripe customer on the `user`), so it keeps the login gate.

## What's implemented (works today)

USD lane — wired to the Rails `stripe_ai_billing` endpoints (in the treechat
backend; the CLI calls them via `profile.BackendURL`):
- `treecli billing checkout` (alias `setup`) → `POST /api/v1/stripe_ai_billing/checkout` → opens `{url}`
- `treecli billing status [--refresh] [--json]` → `GET /api/v1/stripe_ai_billing/status`
- `treecli billing mode <usd|bsv>` → `POST /api/v1/stripe_ai_billing/mode` (`stripe_metered|bsv`; reuses `normalizePaymentMode`)
- `treecli billing sync <session-id>` → `GET /api/v1/stripe_ai_billing/sync?session_id=`

BSV lane — local, no login (`wallet` package + `cmd/billing_wallet.go`):
- `treecli billing wallet new [label] [--show-secret]` — generate a Shuallet-style keyfile (`payPk` + `ordPk`), store keyfile (0600)
- `treecli billing wallet import <file> [--label]` — tolerant parse of `treechat_shuallet.json` / any `{payPk|wif|...}` JSON, re-derives + cross-checks payment/ordinals addresses
- `treecli billing wallet address [name]` / `list` — deposit address / inventory
- Keyfiles live in `<xdg config>/treecli/wallets/*.json`, using Treechat's Shuallet fields (`payPk` for payments, `ordPk` for ordinals)

Keygen/import/address are native Go via `github.com/bsv-blockchain/go-sdk` — WIF
and P2PKH are standard, so imported Treechat/Shuallet payment keys derive the
same payment address in the CLI and the web app.

## What's left for Codex

### 1. `treecli billing wallet pay` — on-chain settlement (the main piece)
Currently a preview stub (`runBillingWalletPay`). To wire it:
- **Outstanding amount**: from `GET /stripe_ai_billing/status` → `usage.unpaid_total_cents`, or the BSV-mode equivalent. Confirm how BSV-mode charges are represented (backend `app/models/ai_fee_charge.rb`, `app/services/ai_billing_router.rb`, `app/controllers/api/v1/bsv_fee_charges_controller.rb`).
- **Recipient**: `TreechatConfig.revenue_wallet_address` (referenced in the backend `bsv_controller.rb`).
- **Build + sign**: `go-sdk` has `transaction` + `template/p2pkh` and `ec.PrivateKeyFromWif`; sign locally with the wallet WIF. Fetch UTXOs (backend `bsv` utxos/balance, or `BsvJsApi`).
- **Broadcast**: `POST /api/v1/bsv/broadcast_tx {raw_tx, fund, pay_pk}` or via `BsvJsApi::Client.broadcast_tx`. Also see `bsv/create_fragment_wallet_tx`.
- Confirm whether paying from a *local* wallet must also credit the account's usage ledger, or whether the local lane is purely accountless.

The web wallet is a **2-of-2 split key** (treechat `app/javascript/wallets/wallets.ts`:
`splitKey`/`recombineShares`, signing in `lib/bsv-signing.ts`). go-sdk mirrors
this (`ec.PrivateKey.ToKeyShares` / `PrivateKeyFromKeyShares`) if the local lane
should interoperate with server-held shares rather than a standalone WIF.

### 2. Auto-pay (BSV default)
Design calls for auto-pay on by default. With a local WIF wallet that means the
CLI signs charges without prompting, capped by balance. Decide the unattended
model: sign-on-next-invocation vs a persistent signer vs a pre-signed mandate.

## Review decisions / risks (please confirm)
1. **Encryption at rest** — keyfiles are 0600 plaintext (same as the web
   `treechat_shuallet.json` download). Consider passphrase / OS-keychain encryption
   for the CLI keystore.

## Files
- `cmd/billing.go` — USD lane commands + shared helpers
- `cmd/billing_wallet.go` — BSV local-wallet lane commands
- `api/stripe_ai_billing.go` — Stripe billing API client + types
- `wallet/wallet.go` — keygen / import / keyfile storage (+ `wallet_test.go`)
- `treecli.go` — registers `BillingCmd`
