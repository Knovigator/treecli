package api

import (
	"encoding/json"
	"fmt"
	"net/http"
)

// StripeAiBillingCheckoutResponse is returned by POST /api/v1/stripe_ai_billing/checkout.
type StripeAiBillingCheckoutResponse struct {
	URL string          `json:"url"`
	Raw json.RawMessage `json:"-"`
}

// StripeAiBillingStatusResponse is returned by GET /api/v1/stripe_ai_billing/status.
type StripeAiBillingStatusResponse struct {
	Stripe StripeAiBillingStripe `json:"stripe"`
	Usage  StripeAiBillingUsage  `json:"usage"`
	Raw    json.RawMessage       `json:"-"`
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
	ID              int64  `json:"id"`
	AmountCents     int64  `json:"amount_cents"`
	Status          string `json:"status"`
	OccurredAt      string `json:"occurred_at"`
	StripeInvoiceID string `json:"stripe_invoice_id"`
	LastError       string `json:"last_error"`
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

func CreateStripeAiBillingCheckoutSession(backendURL, accessToken, client, uid string) (StripeAiBillingCheckoutResponse, error) {
	resp, err := newRequest(accessToken, client, uid).
		SetHeader("accept", "application/json").
		Post(fmt.Sprintf("%s/api/v1/stripe_ai_billing/checkout", backendURL))
	if err != nil {
		return StripeAiBillingCheckoutResponse{}, fmt.Errorf("error making request: %v", err)
	}

	if resp.StatusCode() != http.StatusOK {
		return StripeAiBillingCheckoutResponse{}, fmt.Errorf("received status code %d: %s", resp.StatusCode(), resp.Body())
	}

	var checkout StripeAiBillingCheckoutResponse
	if err := json.Unmarshal(resp.Body(), &checkout); err != nil {
		return StripeAiBillingCheckoutResponse{}, fmt.Errorf("error parsing response: %v", err)
	}
	checkout.Raw = append(checkout.Raw[:0], resp.Body()...)

	return checkout, nil
}

func GetStripeAiBillingStatus(backendURL, accessToken, client, uid string, refresh bool) (StripeAiBillingStatusResponse, error) {
	request := newRequest(accessToken, client, uid).
		SetHeader("accept", "application/json")
	if refresh {
		request.SetQueryParam("refresh", "true")
	}

	resp, err := request.Get(fmt.Sprintf("%s/api/v1/stripe_ai_billing/status", backendURL))
	if err != nil {
		return StripeAiBillingStatusResponse{}, fmt.Errorf("error making request: %v", err)
	}

	if resp.StatusCode() != http.StatusOK {
		return StripeAiBillingStatusResponse{}, fmt.Errorf("received status code %d: %s", resp.StatusCode(), resp.Body())
	}

	var status StripeAiBillingStatusResponse
	if err := json.Unmarshal(resp.Body(), &status); err != nil {
		return StripeAiBillingStatusResponse{}, fmt.Errorf("error parsing response: %v", err)
	}
	status.Raw = append(status.Raw[:0], resp.Body()...)

	return status, nil
}

func SetStripeAiPaymentMode(backendURL, accessToken, client, uid, mode string) (StripeAiPaymentModeResponse, error) {
	resp, err := newRequest(accessToken, client, uid).
		SetHeader("accept", "application/json").
		SetFormData(map[string]string{"mode": mode}).
		Post(fmt.Sprintf("%s/api/v1/stripe_ai_billing/mode", backendURL))
	if err != nil {
		return StripeAiPaymentModeResponse{}, fmt.Errorf("error making request: %v", err)
	}

	if resp.StatusCode() != http.StatusOK {
		return StripeAiPaymentModeResponse{}, fmt.Errorf("received status code %d: %s", resp.StatusCode(), resp.Body())
	}

	var result StripeAiPaymentModeResponse
	if err := json.Unmarshal(resp.Body(), &result); err != nil {
		return StripeAiPaymentModeResponse{}, fmt.Errorf("error parsing response: %v", err)
	}
	result.Raw = append(result.Raw[:0], resp.Body()...)

	return result, nil
}

func SyncStripeAiBillingCheckoutSession(backendURL, accessToken, client, uid, sessionID string) (StripeAiBillingSyncResponse, error) {
	resp, err := newRequest(accessToken, client, uid).
		SetHeader("accept", "application/json").
		SetQueryParam("session_id", sessionID).
		Get(fmt.Sprintf("%s/api/v1/stripe_ai_billing/sync", backendURL))
	if err != nil {
		return StripeAiBillingSyncResponse{}, fmt.Errorf("error making request: %v", err)
	}

	if resp.StatusCode() != http.StatusOK {
		return StripeAiBillingSyncResponse{}, fmt.Errorf("received status code %d: %s", resp.StatusCode(), resp.Body())
	}

	var result StripeAiBillingSyncResponse
	if err := json.Unmarshal(resp.Body(), &result); err != nil {
		return StripeAiBillingSyncResponse{}, fmt.Errorf("error parsing response: %v", err)
	}
	result.Raw = append(result.Raw[:0], resp.Body()...)

	return result, nil
}
