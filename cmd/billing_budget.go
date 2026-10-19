package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/Knovigator/treecli/api"
	"github.com/spf13/cobra"
)

var billingBudgetJSON bool

var billingBudgetCmd = &cobra.Command{
	Use: "budget", Short: "Show the connected application AI cap over the last 24 hours",
	Long: "Inspect the connected application's AI budget in USD cents over a rolling 24-hour window. Change the daily limit by reconnecting with treecli connect and approving browser consent.",
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runBillingBudget(cmd)
	},
}

func init() {
	billingBudgetCmd.PersistentFlags().BoolVar(&billingBudgetJSON, "json", false, "Output the backend budget and accounting details as JSON")
	BillingCmd.AddCommand(billingBudgetCmd)
}

func runBillingBudget(cmd *cobra.Command) error {
	profile, err := requireAuthenticatedProfile()
	if err != nil {
		return err
	}
	raw, err := api.SharedOperation(profile.BackendURL, profile.AccessToken, profile.Client, profile.UID, "get_generation_budget", map[string]interface{}{})
	if err != nil {
		return err
	}
	if billingBudgetJSON {
		fmt.Fprintln(cmd.OutOrStdout(), string(raw))
		return nil
	}
	var budget struct {
		Daily         *int64          `json:"daily_limit_cents"`
		Reserved      int64           `json:"reserved_cents"`
		Remaining     *int64          `json:"remaining_cents"`
		Currency      string          `json:"currency"`
		Status        string          `json:"status"`
		NextReleaseAt *string         `json:"next_release_at"`
		Scope         string          `json:"scope"`
		Accounting    json.RawMessage `json:"accounting"`
		Exclusions    json.RawMessage `json:"exclusions"`
	}
	if err := json.Unmarshal(raw, &budget); err != nil {
		return err
	}
	out := cmd.OutOrStdout()
	if budget.Status == "not_connected" {
		fmt.Fprintln(out, "Connection: not connected")
		fmt.Fprintln(out, "Daily cap: unset; run treecli connect to approve an application limit")
	} else if budget.Daily == nil {
		fmt.Fprintln(out, "Daily cap: unset")
	} else {
		fmt.Fprintf(out, "Daily cap: %d %s cents\n", *budget.Daily, budget.Currency)
	}
	fmt.Fprintf(out, "Reserved: %d %s cents\n", budget.Reserved, budget.Currency)
	if budget.Remaining == nil {
		fmt.Fprintln(out, "Remaining: unavailable")
	} else {
		fmt.Fprintf(out, "Remaining: %d %s cents\n", *budget.Remaining, budget.Currency)
	}
	fmt.Fprintln(out, "Window: rolling 24 hours (no fixed midnight reset)")
	if budget.NextReleaseAt != nil {
		fmt.Fprintf(out, "Next capacity release (UTC): %s\n", *budget.NextReleaseAt)
	} else {
		fmt.Fprintln(out, "Next capacity release: none scheduled")
	}
	fmt.Fprintf(out, "Scope: %s\n", budget.Scope)
	fmt.Fprintln(out, "To change the daily limit, run treecli connect and approve browser consent.")
	if len(budget.Accounting) > 0 {
		fmt.Fprintf(out, "Accounting: %s\n", budget.Accounting)
	}
	if len(budget.Exclusions) > 0 {
		fmt.Fprintf(out, "Exclusions: %s\n", budget.Exclusions)
	}
	return nil
}
