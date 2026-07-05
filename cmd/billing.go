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

// BillingCmd is the parent for both billing lanes:
//   - USD (Stripe): account-tied, requires `treecli login`.
//   - BSV: a local wallet pays directly, no login required (see `billing wallet`).
var BillingCmd = &cobra.Command{
	Use:   "billing",
	Short: "Manage AI usage billing (USD via Stripe, or BSV via a local wallet)",
	Long: `Manage how Treechat charges you for AI usage.

Two lanes:
  USD  Stripe card on file, charged pay-as-you-go. Account-tied: requires
       ` + "`treecli login`" + `. See: billing checkout | status | mode | sync.
  BSV  A local treechat-style wallet pays directly from its balance. No login
       required. See: billing wallet new | import | address.`,
}

var billingCheckoutCmd = &cobra.Command{
	Use:     "checkout",
	Aliases: []string{"setup"},
	Short:   "Open Stripe Checkout for USD AI billing (requires login)",
	Long:    `Create a Stripe Checkout session for metered USD AI billing and open it in your browser.`,
	Example: "  treecli billing checkout\n  treecli billing setup\n  treecli billing checkout --open=false",
	Run:     runBillingCheckout,
}

var billingStatusCmd = &cobra.Command{
	Use:     "status",
	Short:   "Show AI billing status and recent usage (requires login)",
	Example: "  treecli billing status\n  treecli billing status --refresh\n  treecli billing status --json",
	Run:     runBillingStatus,
}

var billingModeCmd = &cobra.Command{
	Use:     "mode <usd|bsv>",
	Short:   "Set the default AI payment mode (requires login)",
	Example: "  treecli billing mode usd\n  treecli billing mode bsv",
	Args:    cobra.ExactArgs(1),
	Run:     runBillingMode,
}

var billingSyncCmd = &cobra.Command{
	Use:     "sync <checkout-session-id>",
	Short:   "Sync billing after returning from Stripe Checkout (requires login)",
	Example: "  treecli billing sync cs_test_...",
	Args:    cobra.ExactArgs(1),
	Run:     runBillingSync,
}

func init() {
	billingCheckoutCmd.Flags().BoolVar(&billingCheckoutOpen, "open", true, "Open the checkout URL in the default browser")
	billingCheckoutCmd.Flags().BoolVar(&billingCheckoutJSON, "json", false, "Output JSON instead of human-readable text")
	billingStatusCmd.Flags().BoolVar(&billingStatusRefresh, "refresh", false, "Ask the backend to refresh Stripe subscription and invoice state")
	billingStatusCmd.Flags().BoolVar(&billingStatusJSON, "json", false, "Output JSON instead of human-readable text")
	billingModeCmd.Flags().BoolVar(&billingModeJSON, "json", false, "Output JSON instead of human-readable text")
	billingSyncCmd.Flags().BoolVar(&billingSyncJSON, "json", false, "Output JSON instead of human-readable text")

	BillingCmd.AddCommand(billingCheckoutCmd)
	BillingCmd.AddCommand(billingStatusCmd)
	BillingCmd.AddCommand(billingModeCmd)
	BillingCmd.AddCommand(billingSyncCmd)
	BillingCmd.AddCommand(billingWalletCmd)
}

func runBillingCheckout(cmd *cobra.Command, args []string) {
	profile, err := requireAuthenticatedProfile()
	if err != nil {
		fmt.Println("Error:", err)
		return
	}

	checkout, err := api.CreateStripeAiBillingCheckoutSession(
		profile.BackendURL,
		profile.AccessToken,
		profile.Client,
		profile.UID,
	)
	if err != nil {
		fmt.Println("Error creating checkout session:", err)
		return
	}

	if billingCheckoutJSON {
		printRawOrJSON(checkout.Raw, checkout)
		return
	}

	if strings.TrimSpace(checkout.URL) == "" {
		fmt.Println("Error: checkout response did not include a URL")
		return
	}

	fmt.Printf("Checkout URL: %s\n", checkout.URL)
	if billingCheckoutOpen {
		if err := openExternalURL(checkout.URL); err != nil {
			fmt.Printf("Could not open browser: %v\n", err)
			fmt.Println("Open the URL above manually to finish card setup.")
		} else {
			fmt.Println("Opened checkout in your browser.")
		}
	}
	fmt.Println("After checkout, Treechat should sync automatically when the web app returns.")
	fmt.Println("Run `treecli billing status --refresh` to verify.")
}

func runBillingStatus(cmd *cobra.Command, args []string) {
	profile, err := requireAuthenticatedProfile()
	if err != nil {
		fmt.Println("Error:", err)
		return
	}

	status, err := api.GetStripeAiBillingStatus(
		profile.BackendURL,
		profile.AccessToken,
		profile.Client,
		profile.UID,
		billingStatusRefresh,
	)
	if err != nil {
		fmt.Println("Error loading billing status:", err)
		return
	}

	if billingStatusJSON {
		printRawOrJSON(status.Raw, status)
		return
	}

	printBillingStatus(status)
}

func runBillingMode(cmd *cobra.Command, args []string) {
	mode, err := normalizePaymentMode(args[0])
	if err != nil {
		fmt.Println("Error:", err)
		return
	}
	if mode == "" {
		fmt.Println("Error: specify a payment mode: usd or bsv")
		return
	}

	profile, err := requireAuthenticatedProfile()
	if err != nil {
		fmt.Println("Error:", err)
		return
	}

	result, err := api.SetStripeAiPaymentMode(
		profile.BackendURL,
		profile.AccessToken,
		profile.Client,
		profile.UID,
		mode,
	)
	if err != nil {
		fmt.Println("Error setting billing mode:", err)
		return
	}

	if billingModeJSON {
		printRawOrJSON(result.Raw, result)
		return
	}

	fmt.Printf("AI payment mode: %s\n", paymentModeDisplay(result.AIPaymentMode))
}

func runBillingSync(cmd *cobra.Command, args []string) {
	sessionID := strings.TrimSpace(args[0])
	if sessionID == "" {
		fmt.Println("Error: checkout session id is required.")
		return
	}

	profile, err := requireAuthenticatedProfile()
	if err != nil {
		fmt.Println("Error:", err)
		return
	}

	result, err := api.SyncStripeAiBillingCheckoutSession(
		profile.BackendURL,
		profile.AccessToken,
		profile.Client,
		profile.UID,
		sessionID,
	)
	if err != nil {
		fmt.Println("Error syncing checkout session:", err)
		return
	}

	if billingSyncJSON {
		printRawOrJSON(result.Raw, result)
		return
	}

	if result.OK {
		fmt.Println("Billing synced.")
		fmt.Println("Run `treecli billing status` to verify the active payment mode.")
		return
	}

	fmt.Println("Billing sync completed without an ok response.")
}

func printBillingStatus(status api.StripeAiBillingStatusResponse) {
	fmt.Printf("AI payment mode: %s\n", displayValue(paymentModeDisplay(status.Stripe.AIPaymentMode)))
	fmt.Printf("Stripe subscription: %s\n", displayValue(status.Stripe.SubscriptionStatus))
	if status.Stripe.CustomerID != "" {
		fmt.Printf("Customer: %s\n", status.Stripe.CustomerID)
	}
	if status.Stripe.SubscriptionID != "" {
		fmt.Printf("Subscription: %s\n", status.Stripe.SubscriptionID)
	}
	if status.Stripe.MeteredLockedAt != "" {
		fmt.Printf("Metered billing locked at: %s\n", status.Stripe.MeteredLockedAt)
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
		line := fmt.Sprintf("  #%d %s %s", event.ID, event.Status, centsToUSD(event.AmountCents))
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
