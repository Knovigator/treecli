package cmd

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"runtime"
	"sort"
	"strings"

	"github.com/Knovigator/treecli/api"
	"github.com/spf13/cobra"
)

var billingStatusRefresh bool
var billingStatusJSON bool
var billingCheckoutOpen bool
var billingCheckoutJSON bool
var billingSyncJSON bool
var billingModeJSON bool
var billingCheckoutAmount int
var billingAutoReloadAmount int
var billingAutoReloadBelow int
var billingAutoReloadJSON bool

// BillingCmd is the parent for both billing lanes:
//   - USD (Stripe): account-tied, requires `treecli login`.
//   - BSV: a local wallet pays directly, no login required (see `billing wallet`).
var BillingCmd = &cobra.Command{
	Use:   "billing",
	Short: "Manage AI usage billing (USD via Stripe, or BSV via a local wallet)",
	Long: `Manage how Treechat charges you for AI usage.

Two lanes:
  USD  Account-tied: requires ` + "`treecli connect`" + ` (or login). Prepaid credits
       bought with a card, topped up automatically when low; accounts that
       already have card billing are charged pay as you go instead.
       See: billing checkout | status | auto-reload | mode | sync.
  BSV  A local treechat-style wallet pays directly from its balance. No login
       required. See: billing wallet new | import | address.`,
}

var billingCheckoutCmd = &cobra.Command{
	Use:     "checkout",
	Aliases: []string{"setup"},
	Short:   "Buy AI credits (or set up card billing) in Stripe Checkout",
	Long: `Create a Stripe Checkout session and open it in your browser.

With prepaid credits (the default) this buys credits; --amount picks how many
dollars (5, 10, 20 or 50). The card is saved for auto-reload. Accounts on card
billing set up their card instead and are charged pay as you go.`,
	Example: "  treecli billing checkout\n  treecli billing checkout --amount 20\n  treecli billing checkout --open=false",
	RunE:    runBillingCheckout,
}

var billingStatusCmd = &cobra.Command{
	Use:     "status",
	Short:   "Show AI billing status and recent usage (requires login)",
	Example: "  treecli billing status\n  treecli billing status --refresh\n  treecli billing status --json",
	RunE:    runBillingStatus,
}

var billingModeCmd = &cobra.Command{
	Use:     "mode <usd|bsv>",
	Short:   "Set the default AI payment mode (requires login)",
	Example: "  treecli billing mode usd\n  treecli billing mode bsv",
	Args:    cobra.ExactArgs(1),
	RunE:    runBillingMode,
}

var billingAutoReloadCmd = &cobra.Command{
	Use:   "auto-reload [on|off]",
	Short: "Turn credits auto-reload on or off, or change its amount (requires login)",
	Long: `When your AI credit balance drops below a threshold, auto-reload charges the
card saved at checkout and adds credits. --amount is the dollars added (10, 20
or 50); --below is the balance in dollars that triggers it (2, 5 or 10).`,
	Example: "  treecli billing auto-reload off\n  treecli billing auto-reload on --amount 50 --below 10\n  treecli billing auto-reload --amount 10",
	Args:    cobra.MaximumNArgs(1),
	RunE:    runBillingAutoReload,
}

var billingSyncCmd = &cobra.Command{
	Use:     "sync <checkout-session-id>",
	Short:   "Sync billing after returning from Stripe Checkout (requires login)",
	Example: "  treecli billing sync cs_test_...",
	Args:    cobra.ExactArgs(1),
	RunE:    runBillingSync,
}

func init() {
	billingCheckoutCmd.Flags().BoolVar(&billingCheckoutOpen, "open", true, "Open the checkout URL in the default browser")
	billingCheckoutCmd.Flags().BoolVar(&billingCheckoutJSON, "json", false, "Output JSON instead of human-readable text")
	billingStatusCmd.Flags().BoolVar(&billingStatusRefresh, "refresh", false, "Ask the backend to refresh Stripe subscription and invoice state")
	billingStatusCmd.Flags().BoolVar(&billingStatusJSON, "json", false, "Output JSON instead of human-readable text")
	billingModeCmd.Flags().BoolVar(&billingModeJSON, "json", false, "Output JSON instead of human-readable text")
	billingSyncCmd.Flags().BoolVar(&billingSyncJSON, "json", false, "Output JSON instead of human-readable text")
	billingCheckoutCmd.Flags().IntVar(&billingCheckoutAmount, "amount", 0, "Dollars of AI credits to buy: 5, 10, 20 or 50 (credits accounts)")
	billingAutoReloadCmd.Flags().IntVar(&billingAutoReloadAmount, "amount", 0, "Dollars to add on each reload: 10, 20 or 50")
	billingAutoReloadCmd.Flags().IntVar(&billingAutoReloadBelow, "below", 0, "Reload when the balance drops below this many dollars: 2, 5 or 10")
	billingAutoReloadCmd.Flags().BoolVar(&billingAutoReloadJSON, "json", false, "Output JSON instead of human-readable text")

	BillingCmd.AddCommand(billingCheckoutCmd)
	BillingCmd.AddCommand(billingStatusCmd)
	BillingCmd.AddCommand(billingModeCmd)
	BillingCmd.AddCommand(billingSyncCmd)
	BillingCmd.AddCommand(billingAutoReloadCmd)
	BillingCmd.AddCommand(billingWalletCmd)
}

func runBillingCheckout(cmd *cobra.Command, args []string) error {
	if billingCheckoutAmount != 0 && !containsInt([]int{5, 10, 20, 50}, billingCheckoutAmount) {
		return usageErrorf("--amount must be 5, 10, 20 or 50 (dollars)")
	}
	profile, err := requireAuthenticatedProfile()
	if err != nil {
		return err
	}
	checkout, err := api.CreateStripeAiBillingCheckoutSession(
		profile.BackendURL,
		profile.AccessToken,
		profile.Client,
		profile.UID,
		billingCheckoutAmount*100,
	)
	if err != nil {
		return fmt.Errorf("creating checkout session: %w", err)
	}

	if billingCheckoutJSON {
		printRawOrJSON(checkout.Raw, checkout)
		return nil
	}

	if strings.TrimSpace(checkout.URL) == "" {
		return fmt.Errorf("checkout response did not include a URL")
	}

	fmt.Printf("Checkout URL: %s\n", checkout.URL)
	if billingCheckoutOpen {
		if err := openExternalURL(checkout.URL); err != nil {
			fmt.Printf("Could not open browser: %v\n", err)
			fmt.Println("Open the URL above manually to finish checkout.")
		} else {
			fmt.Println("Opened checkout in your browser.")
		}
	}
	if checkout.Mode == "credits" {
		fmt.Println("Credits are added as soon as the payment goes through.")
	} else {
		fmt.Println("After checkout, Treechat should sync automatically when the web app returns.")
	}
	fmt.Println("Run `treecli billing status` to verify.")
	return nil
}

func runBillingAutoReload(cmd *cobra.Command, args []string) error {
	var enabled *bool
	if len(args) == 1 {
		switch strings.ToLower(args[0]) {
		case "on":
			on := true
			enabled = &on
		case "off":
			off := false
			enabled = &off
		default:
			return usageErrorf("say on or off, e.g. `treecli billing auto-reload off`")
		}
	}
	if billingAutoReloadAmount != 0 && !containsInt([]int{10, 20, 50}, billingAutoReloadAmount) {
		return usageErrorf("--amount must be 10, 20 or 50 (dollars)")
	}
	if billingAutoReloadBelow != 0 && !containsInt([]int{2, 5, 10}, billingAutoReloadBelow) {
		return usageErrorf("--below must be 2, 5 or 10 (dollars)")
	}
	if enabled == nil && billingAutoReloadAmount == 0 && billingAutoReloadBelow == 0 {
		return usageErrorf("say on or off, or pass --amount / --below")
	}

	profile, err := requireAuthenticatedProfile()
	if err != nil {
		return err
	}
	result, err := api.SetAutoReload(
		profile.BackendURL,
		profile.AccessToken,
		profile.Client,
		profile.UID,
		enabled,
		billingAutoReloadAmount*100,
		billingAutoReloadBelow*100,
	)
	if err != nil {
		return fmt.Errorf("updating auto-reload: %w", err)
	}
	if billingAutoReloadJSON {
		printRawOrJSON(result.Raw, result)
		return nil
	}
	if result.Credits != nil {
		printAutoReload(*result.Credits)
	}
	return nil
}

func containsInt(values []int, value int) bool {
	for _, candidate := range values {
		if candidate == value {
			return true
		}
	}
	return false
}

func runBillingStatus(cmd *cobra.Command, args []string) error {
	profile, err := requireAuthenticatedProfile()
	if err != nil {
		return err
	}

	status, err := api.GetStripeAiBillingStatus(
		profile.BackendURL,
		profile.AccessToken,
		profile.Client,
		profile.UID,
		billingStatusRefresh,
	)
	if err != nil {
		return fmt.Errorf("loading billing status: %w", err)
	}

	if billingStatusJSON {
		printRawOrJSON(status.Raw, status)
		return nil
	}

	printBillingStatus(status)
	return nil
}

func runBillingMode(cmd *cobra.Command, args []string) error {
	mode, err := normalizePaymentMode(args[0])
	if err != nil {
		return err
	}
	if mode == "" {
		return usageErrorf("specify a payment mode: usd or bsv")
	}

	profile, err := requireAuthenticatedProfile()
	if err != nil {
		return err
	}

	result, err := api.SetStripeAiPaymentMode(
		profile.BackendURL,
		profile.AccessToken,
		profile.Client,
		profile.UID,
		mode,
	)
	if err != nil {
		return fmt.Errorf("setting billing mode: %w", err)
	}

	if billingModeJSON {
		printRawOrJSON(result.Raw, result)
		return nil
	}

	fmt.Printf("AI payment mode: %s\n", paymentModeDisplay(result.AIPaymentMode))
	return nil
}

func runBillingSync(cmd *cobra.Command, args []string) error {
	sessionID := strings.TrimSpace(args[0])
	if sessionID == "" {
		return usageErrorf("checkout session id is required")
	}

	profile, err := requireAuthenticatedProfile()
	if err != nil {
		return err
	}

	result, err := api.SyncStripeAiBillingCheckoutSession(
		profile.BackendURL,
		profile.AccessToken,
		profile.Client,
		profile.UID,
		sessionID,
	)
	if err != nil {
		return fmt.Errorf("syncing checkout session: %w", err)
	}

	if billingSyncJSON {
		printRawOrJSON(result.Raw, result)
		return nil
	}

	if result.OK {
		fmt.Println("Billing synced.")
		fmt.Println("Run `treecli billing status` to verify the active payment mode.")
		return nil
	}

	return fmt.Errorf("billing sync completed without an ok response")
}

func printBillingStatus(status api.StripeAiBillingStatusResponse) {
	fmt.Printf("AI payment mode: %s\n", displayValue(paymentModeDisplay(status.Stripe.AIPaymentMode)))
	if status.USD != nil && status.USD.Mode == "credits" && status.USD.Credits != nil {
		printCreditsStatus(*status.USD.Credits)
		return
	}
	fmt.Printf("Card billing (pay as you go): %s\n", displayValue(status.Stripe.SubscriptionStatus))
	if status.Stripe.CustomerID != "" {
		fmt.Printf("Customer: %s\n", status.Stripe.CustomerID)
	}
	if status.Stripe.SubscriptionID != "" {
		fmt.Printf("Subscription: %s\n", status.Stripe.SubscriptionID)
	}
	if status.Stripe.MeteredLockedAt != "" {
		fmt.Printf("Card billing locked at: %s\n", status.Stripe.MeteredLockedAt)
	}
	if status.Stripe.MeteredLockReason != "" {
		fmt.Printf("Lock reason: %s\n", status.Stripe.MeteredLockReason)
	}
	if status.Stripe.RefreshedAt != "" {
		fmt.Printf("Last Stripe refresh: %s\n", status.Stripe.RefreshedAt)
	}

	fmt.Printf("Outstanding usage: %s\n", centsToUSD(status.Usage.UnpaidTotalCents))
	if len(status.Usage.TotalsCentsByStatus) > 0 {
		fmt.Println("Usage totals:")
		statuses := make([]string, 0, len(status.Usage.TotalsCentsByStatus))
		for statusName := range status.Usage.TotalsCentsByStatus {
			statuses = append(statuses, statusName)
		}
		sort.Strings(statuses)
		for _, statusName := range statuses {
			fmt.Printf("  %s: %s\n", statusName, centsToUSD(status.Usage.TotalsCentsByStatus[statusName]))
		}
	}

	if len(status.Usage.Events) == 0 {
		return
	}

	fmt.Println("Recent usage:")
	for index, event := range status.Usage.Events {
		if index >= 10 {
			break
		}
		line := fmt.Sprintf("  #%s %s %s", event.ID, event.Status, centsToUSD(event.AmountCents))
		if event.OccurredAt != "" {
			line += " " + event.OccurredAt
		}
		if event.StripeInvoiceID != "" {
			line += " invoice=" + event.StripeInvoiceID
		}
		if event.LastError != "" {
			line += " error=" + event.LastError
		}
		fmt.Println(line)
	}
}

func printCreditsStatus(credits api.AiCreditsStatus) {
	fmt.Printf("AI credits: %s\n", centsToUSD(credits.BalanceCents))
	if credits.Card != "" {
		fmt.Printf("Card: %s\n", credits.Card)
	}
	printAutoReload(credits)
	shown := 0
	for _, entry := range credits.Entries {
		if shown == 0 {
			fmt.Println("Recent activity:")
		}
		if shown >= 10 {
			break
		}
		shown++
		line := fmt.Sprintf("  %s %s", creditEntryLabel(entry), centsToUSD(entry.AmountCents))
		if entry.CreatedAt != "" {
			line += " " + entry.CreatedAt
		}
		fmt.Println(line)
	}
	if credits.BalanceCents <= 0 && !credits.CardSaved {
		fmt.Println("Buy credits with `treecli billing checkout --amount 10`.")
	}
}

func printAutoReload(credits api.AiCreditsStatus) {
	switch {
	case credits.ReloadFailedAt != "":
		reason := credits.ReloadFailure
		if reason == "" {
			reason = "the card was declined"
		}
		fmt.Printf("Auto-reload: paused (%s). Run `treecli billing auto-reload on` or buy credits to retry.\n", reason)
	case !credits.AutoReload:
		fmt.Println("Auto-reload: off")
	case !credits.CardSaved:
		fmt.Printf("Auto-reload: on, starts after your first purchase saves a card (adds %s below %s)\n", centsToUSD(credits.ReloadCents), centsToUSD(credits.ThresholdCents))
	default:
		fmt.Printf("Auto-reload: on, adds %s when the balance drops below %s\n", centsToUSD(credits.ReloadCents), centsToUSD(credits.ThresholdCents))
	}
}

func creditEntryLabel(entry api.AiCreditEntry) string {
	switch entry.Kind {
	case "topup":
		return "added"
	case "hold":
		switch entry.Status {
		case "captured":
			return "generation"
		case "released":
			return "reserved, returned"
		}
		return "reserved"
	case "charge":
		return "generation"
	case "release":
		return "returned"
	}
	return entry.Kind
}

func centsToUSD(cents int64) string {
	sign := ""
	if cents < 0 {
		sign = "-"
		cents = -cents
	}

	return fmt.Sprintf("%s$%d.%02d", sign, cents/100, cents%100)
}

func displayValue(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "not set"
	}

	return value
}

func printRawOrJSON(raw []byte, value interface{}) {
	if len(raw) > 0 {
		pretty, err := api.PrettyJSON(raw)
		if err == nil {
			fmt.Println(pretty)
			return
		}
	}

	formatted, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		fmt.Printf("Error formatting JSON: %v\n", err)
		return
	}
	fmt.Println(string(formatted))
}

func openExternalURL(rawURL string) error {
	switch runtime.GOOS {
	case "darwin":
		return exec.Command("open", rawURL).Start()
	case "windows":
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", rawURL).Start()
	default:
		return exec.Command("xdg-open", rawURL).Start()
	}
}
