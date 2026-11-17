package api

import (
	"encoding/json"
)

// StripeAiBillingCheckoutResponse is returned by POST /api/v1/stripe_ai_billing/checkout.
type StripeAiBillingCheckoutResponse struct {
	URL  string          `json:"url"`
	Mode string          `json:"mode"`
	Raw  json.RawMessage `json:"-"`
}

// StripeAiBillingStatusResponse is returned by GET /api/v1/stripe_ai_billing/status.
type StripeAiBillingStatusResponse struct {
	USD    *StripeAiBillingUSD   `json:"usd"`
	Stripe StripeAiBillingStripe `json:"stripe"`
	Usage  StripeAiBillingUsage  `json:"usage"`
	Raw    json.RawMessage       `json:"-"`
}

// StripeAiBillingUSD says how USD is collected: "credits" (a prepaid balance
// with auto-reload) or "settlement" (card billing, pay as you go). Older
// servers leave it out.
type StripeAiBillingUSD struct {
	Mode    string           `json:"mode"`
	Payable bool             `json:"payable"`
	Credits *AiCreditsStatus `json:"credits"`
}

type AiCreditsStatus struct {
	BalanceCents      int64           `json:"balance_cents"`
	AutoReload        bool            `json:"auto_reload"`
	ReloadCents       int64           `json:"reload_cents"`
	ThresholdCents    int64           `json:"threshold_cents"`
	Card              string          `json:"card"`
	CardSaved         bool            `json:"card_saved"`
	ReloadFailedAt    string          `json:"reload_failed_at"`
	ReloadFailure     string          `json:"reload_failure"`
	TopupOptionsCents []int64         `json:"topup_options_cents"`
	Entries           []AiCreditEntry `json:"entries"`
}

type AiCreditEntry struct {
	ID                string `json:"id"`
	Kind              string `json:"kind"`
	Status            string `json:"status"`
	AmountCents       int64  `json:"amount_cents"`
	BalanceAfterCents int64  `json:"balance_after_cents"`
	CreatedAt         string `json:"created_at"`
}

// AutoReloadResponse is returned by the treechat_billing auto_reload operation.
type AutoReloadResponse struct {
	OK      bool             `json:"ok"`
	Credits *AiCreditsStatus `json:"credits"`
	Raw     json.RawMessage  `json:"-"`
}

type StripeAiBillingStripe struct {
	CustomerID         string `json:"customer_id"`
	SubscriptionID     string `json:"subscription_id"`
	SubscriptionStatus string `json:"subscription_status"`
	AIPaymentMode      string `json:"ai_payment_mode"`
	MeteredLockedAt    string `json:"metered_locked_at"`
	MeteredLockReason  string `json:"metered_lock_reason"`
	RefreshedAt        string `json:"refreshed_at"`
}

type StripeAiBillingUsage struct {
	UnpaidTotalCents    int64                    `json:"unpaid_total_cents"`
	TotalsCentsByStatus map[string]int64         `json:"totals_cents_by_status"`
	Events              []StripeAiBillingUsEvent `json:"events"`
}

type StripeAiBillingUsEvent struct {
	ID              BillingEventID `json:"id"`
	AmountCents     int64          `json:"amount_cents"`
	Status          string         `json:"status"`
	OccurredAt      string         `json:"occurred_at"`
	StripeInvoiceID string         `json:"stripe_invoice_id"`
	LastError       string         `json:"last_error"`
}

// BillingEventID accepts the backend's UUID strings as well as older numeric IDs.
type BillingEventID string

func (id *BillingEventID) UnmarshalJSON(data []byte) error {
	var text string
	if err := json.Unmarshal(data, &text); err == nil {
		*id = BillingEventID(text)
		return nil
	}
	var number json.Number
	if err := json.Unmarshal(data, &number); err != nil {
		return err
	}
	*id = BillingEventID(number.String())
	return nil
}

// StripeAiPaymentModeResponse is returned by POST /api/v1/stripe_ai_billing/mode.
type StripeAiPaymentModeResponse struct {
	OK            bool            `json:"ok"`
	AIPaymentMode string          `json:"ai_payment_mode"`
	Raw           json.RawMessage `json:"-"`
}

// StripeAiBillingSyncResponse is returned by GET /api/v1/stripe_ai_billing/sync.
type StripeAiBillingSyncResponse struct {
	OK  bool            `json:"ok"`
	Raw json.RawMessage `json:"-"`
}

// CreateStripeAiBillingCheckoutSession starts Checkout: a credit top-up of
// amountCents in credits mode (0 = the server's default), or card setup.
func CreateStripeAiBillingCheckoutSession(backendURL, accessToken, client, uid string, amountCents int) (StripeAiBillingCheckoutResponse, error) {
	var out StripeAiBillingCheckoutResponse
	arguments := map[string]interface{}{}
	if amountCents > 0 {
		arguments["amount_cents"] = amountCents
	}
	raw, err := SharedOperation(backendURL, accessToken, client, uid, "treechat_billing", map[string]interface{}{"operation": "checkout", "arguments": arguments})
	if err == nil {
		err = json.Unmarshal(raw, &out)
	}
	out.Raw = raw
	return out, err
}

func GetStripeAiBillingStatus(backendURL, accessToken, client, uid string, refresh bool) (StripeAiBillingStatusResponse, error) {
	var out StripeAiBillingStatusResponse
	raw, err := SharedOperation(backendURL, accessToken, client, uid, "treechat_billing", map[string]interface{}{"operation": "status", "arguments": map[string]interface{}{"refresh": refresh}})
	if err == nil {
		err = json.Unmarshal(raw, &out)
	}
	out.Raw = raw
	return out, err
}

// SetAutoReload changes credits auto-reload; nil fields stay as they are.
func SetAutoReload(backendURL, accessToken, client, uid string, enabled *bool, reloadCents, thresholdCents int) (AutoReloadResponse, error) {
	var out AutoReloadResponse
	arguments := map[string]interface{}{}
	if enabled != nil {
		arguments["enabled"] = *enabled
	}
	if reloadCents > 0 {
		arguments["reload_cents"] = reloadCents
	}
	if thresholdCents > 0 {
		arguments["threshold_cents"] = thresholdCents
	}
	raw, err := SharedOperation(backendURL, accessToken, client, uid, "treechat_billing", map[string]interface{}{"operation": "auto_reload", "arguments": arguments})
	if err == nil {
		err = json.Unmarshal(raw, &out)
	}
	out.Raw = raw
	return out, err
}

func SetStripeAiPaymentMode(backendURL, accessToken, client, uid, mode string) (StripeAiPaymentModeResponse, error) {
	var out StripeAiPaymentModeResponse
	raw, err := SharedOperation(backendURL, accessToken, client, uid, "treechat_billing", map[string]interface{}{"operation": "mode", "arguments": map[string]interface{}{"mode": mode}})
	if err == nil {
		err = json.Unmarshal(raw, &out)
	}
	out.Raw = raw
	return out, err
}

func SyncStripeAiBillingCheckoutSession(backendURL, accessToken, client, uid, sessionID string) (StripeAiBillingSyncResponse, error) {
	var out StripeAiBillingSyncResponse
	raw, err := SharedOperation(backendURL, accessToken, client, uid, "treechat_billing", map[string]interface{}{"operation": "sync", "arguments": map[string]interface{}{"session_id": sessionID}})
	if err == nil {
		err = json.Unmarshal(raw, &out)
	}
	out.Raw = raw
	return out, err
}
