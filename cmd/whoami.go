package cmd

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

type identityResult struct {
	Environment   string `json:"environment,omitempty"`
	Account       string `json:"account,omitempty"`
	LegacyProfile string `json:"legacy_profile,omitempty"`
	BackendURL    string `json:"backend_url"`
	Username      string `json:"username"`
	UserID        string `json:"user_id"`
	SpaceID       string `json:"-"`
}

var WhoamiCmd = newWhoamiCommand()

func newWhoamiCommand() *cobra.Command {
	var jsonOutput bool
	command := &cobra.Command{
		Use: "whoami", Short: "Verify the selected account's identity with the server", Args: cobra.NoArgs,
		Long:    "Ask the selected server which user the current account credentials authenticate as. Uses the active account and production by default. Unlike account show, this requires a working login and network connection; it never falls back to saved identity data.",
		Example: "  treecli whoami\n  treecli whoami --json\n  treecli --env staging whoami",
		RunE: func(command *cobra.Command, args []string) error {
			profile, err := requireAuthenticatedProfile()
			if err != nil {
				return err
			}
			identity, err := fetchIdentity(profile)
			if err != nil {
				return err
			}
			if jsonOutput {
				data, err := json.MarshalIndent(identity, "", "  ")
				if err != nil {
					return err
				}
				fmt.Fprintln(command.OutOrStdout(), string(data))
				return nil
			}
			if identity.Environment != "" {
				fmt.Fprintf(command.OutOrStdout(), "Environment: %s\nAccount: %s\n", identity.Environment, identity.Account)
			} else {
				fmt.Fprintf(command.OutOrStdout(), "Legacy profile: %s\n", identity.LegacyProfile)
			}
			fmt.Fprintf(command.OutOrStdout(), "Backend: %s\nUsername: %s\nUser ID: %s\n", identity.BackendURL, identity.Username, identity.UserID)
			return nil
		},
	}
	command.Flags().BoolVar(&jsonOutput, "json", false, "Output verified identity as JSON")
	return command
}

func fetchIdentity(profile profileConfig) (identityResult, error) {
	var result struct {
		ID       string `json:"id"`
		Username string `json:"username"`
		SpaceID  string `json:"space_id"`
	}
	if err := sharedCall(profile, "treechat_account", map[string]interface{}{}, &result); err != nil {
		return identityResult{}, fmt.Errorf("could not verify identity: %w", err)
	}
	if strings.TrimSpace(result.ID) == "" || strings.TrimSpace(result.Username) == "" {
		return identityResult{}, fmt.Errorf("could not verify identity: server did not return an authenticated user ID and username; log in again if your session expired")
	}
	identity := identityResult{Environment: profile.Environment, Account: profile.Account, BackendURL: profile.BackendURL, Username: result.Username, UserID: result.ID, SpaceID: result.SpaceID}
	if profile.Environment == "" {
		identity.LegacyProfile = profile.Name
	}
	return identity, nil
}
