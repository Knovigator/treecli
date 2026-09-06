package cmd

import (
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var SelectedEnvironment, SelectedAccount string
var accountSelection bool
var selectionNamePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*$`)

// ConfigureAccountSelection keeps the legacy interface explicit. Old saved
// active_profile values never redirect a new invocation away from production.
func ConfigureAccountSelection(command *cobra.Command, args []string) error {
	legacy := strings.TrimSpace(SelectedProfile) != "" || firstEnv("TREECLI_PROFILE", "TREECTL_PROFILE") != ""
	legacyCommand := false
	accountCommand := false
	for current := command; current != nil; current = current.Parent() {
		if current == AccountCmd {
			accountCommand = true
		}
		if current == ProfileCmd {
			legacyCommand = true
		}
	}
	modern := command.Flags().Changed("env") || command.Flags().Changed("account") || SelectedEnvironment != "" || SelectedAccount != "" || firstEnv("TREECLI_ENV", "TREECLI_ACCOUNT") != ""
	if accountCommand && legacy {
		return fmt.Errorf("account commands require --env/--account, not --profile")
	}
	if (legacy || legacyCommand) && modern {
		return fmt.Errorf("--profile/profile commands cannot be combined with --env or --account; choose one interface")
	}
	if command.Flags().Changed("profile") && strings.TrimSpace(SelectedProfile) == "" {
		return fmt.Errorf("--profile requires a nonempty name")
	}
	for _, name := range []string{"env", "account"} {
		if command.Flags().Changed(name) {
			value, _ := command.Flags().GetString(name)
			if strings.TrimSpace(value) == "" {
				return fmt.Errorf("--%s requires a nonempty name", name)
			}
		}
	}
	accountSelection = !legacy && !legacyCommand
	if !accountSelection {
		if !command.Flags().Changed("profile") {
			fmt.Fprintln(command.ErrOrStderr(), "Deprecated profile selection: use --env ENV --account ACCOUNT. New invocations default to production and never use saved active_profile to choose a server.")
		}
		return nil
	}
	_, _, err := accountNames()
	return err
}

func accountNames() (string, string, error) {
	environment := strings.TrimSpace(SelectedEnvironment)
	if environment == "" {
		environment = firstEnv("TREECLI_ENV")
	}
	environment = normalizeProfileName(environment)
	switch environment {
	case "", "production":
		environment = "prod"
	case "development":
		environment = "dev"
	}
	if !selectionNamePattern.MatchString(environment) {
		return "", "", fmt.Errorf("invalid environment name; use letters, numbers, hyphens or underscores")
	}
	account := strings.TrimSpace(SelectedAccount)
	if account == "" {
		account = firstEnv("TREECLI_ACCOUNT")
	}
	if account == "" {
		account = viper.GetString("environments." + environment + ".active_account")
	}
	if account == "" {
		// Preserve a legacy active identity only when it belongs to this
		// environment. An old dev/staging profile never changes the server.
		legacyName := normalizeProfileName(viper.GetString("active_profile"))
		if selectionNamePattern.MatchString(legacyName) {
			legacy := loadStoredProfile(legacyName)
			if legacy.BackendURL == "" {
				legacy.BackendURL = builtInProfiles()[legacyName].BackendURL
			}
			backend := viper.GetString("environments." + environment + ".backend_url")
			if backend == "" {
				backend = builtInProfiles()[environment].BackendURL
			}
			if backend != "" && normalizeBaseURL(legacy.BackendURL) == normalizeBaseURL(backend) && legacy.AccessToken != "" {
				account = legacyName
				if legacyName == environment {
					account = "default"
				}
			}
		}
	}
	if account == "" {
		account = "default"
	}
	account = normalizeProfileName(account)
	if !selectionNamePattern.MatchString(account) {
		return "", "", fmt.Errorf("invalid account name; use letters, numbers, hyphens or underscores")
	}
	return environment, account, nil
}

func resolveAccount() (profileConfig, error) {
	environment, account, err := accountNames()
	if err != nil {
		return profileConfig{}, err
	}
	return resolveNamedAccount(environment, account)
}

func resolveNamedAccount(environment, account string) (profileConfig, error) {
	server := builtInProfiles()[environment]
	prefix := "environments." + environment + "."
	if value := viper.GetString(prefix + "backend_url"); value != "" {
		server.BackendURL = value
	}
	if value := viper.GetString(prefix + "app_host"); value != "" {
		server.AppHost = value
	}
	if value := strings.TrimSpace(BackendURLOverride); value != "" {
		server.BackendURL = value
	} else if value = firstEnv("TREECLI_BACKEND_URL", "TREECTL_BACKEND_URL"); value != "" {
		server.BackendURL = value
	}
	if value := strings.TrimSpace(AppHostOverride); value != "" {
		server.AppHost = value
	} else if value = firstEnv("TREECLI_APP_HOST", "TREECTL_APP_HOST"); value != "" {
		server.AppHost = value
	}
	server.BackendURL = normalizeBaseURL(server.BackendURL)
	server.AppHost = normalizeAppHost(server.AppHost)
	if server.BackendURL == "" {
		return profileConfig{}, fmt.Errorf("environment %q has no server; use --env %s --backend-url URL --account %s login", environment, environment, account)
	}
	key := "accounts." + environment + "." + account + "."
	stored := profileConfig{BackendURL: viper.GetString(key + "backend_url"), AccessToken: viper.GetString(key + "access_token"), Client: viper.GetString(key + "client"), UID: viper.GetString(key + "uid"), Expiry: viper.GetString(key + "expiry"), CurrentUserID: viper.GetString(key + "current_user_id"), ActiveSpaceID: viper.GetString(key + "active_space_id")}
	if !viper.IsSet(key + "backend_url") {
		legacyName := account
		if account == "default" {
			legacyName = environment
		}
		legacy := loadStoredProfile(legacyName)
		// Built-in legacy profiles may omit the URL in old config files.
		if legacy.BackendURL == "" {
			legacy.BackendURL = builtInProfiles()[legacyName].BackendURL
		}
		if normalizeBaseURL(legacy.BackendURL) == server.BackendURL {
			stored = legacy
		}
	}
	// An override must never forward a saved credential to another server.
	if normalizeBaseURL(stored.BackendURL) != server.BackendURL {
		stored = profileConfig{}
	}
	result := mergeProfile(server, stored)
	result.Name, result.Environment, result.Account = account, environment, account
	result.BackendURL, result.AppHost = server.BackendURL, server.AppHost
	return result, nil
}

var AccountCmd = newAccountCommand()

func newAccountCommand() *cobra.Command {
	command := &cobra.Command{Use: "account", Short: "Inspect and select saved accounts in an environment"}
	var jsonOutput bool
	show := &cobra.Command{Use: "show", Short: "Show the selected environment and account (credentials redacted)", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		profile, err := resolveAccount()
		if err != nil {
			return err
		}
		if jsonOutput {
			data, err := json.MarshalIndent(redactProfile(profile), "", "  ")
			if err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), string(data))
			return nil
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Environment: %s\nAccount: %s\nBackend: %s\nCurrent user id: %s\nCredentials saved: %t\n", profile.Environment, profile.Account, profile.BackendURL, profile.CurrentUserID, profile.AccessToken != "" && profile.Client != "" && profile.UID != "")
		return nil
	}}
	show.Flags().BoolVar(&jsonOutput, "json", false, "Output JSON instead of human-readable text")
	use := &cobra.Command{Use: "use ACCOUNT", Short: "Select the default account for the selected environment", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		environment, _, err := accountNames()
		if err != nil {
			return err
		}
		name := normalizeProfileName(args[0])
		if !selectionNamePattern.MatchString(name) {
			return fmt.Errorf("invalid account name")
		}
		profile, err := resolveNamedAccount(environment, name)
		if err != nil {
			return err
		}
		if profile.AccessToken == "" || profile.Client == "" || profile.UID == "" {
			return fmt.Errorf("account %q has no credentials for environment %q; log in first", name, environment)
		}
		if err := saveProfile(profile, true); err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Active account for %s: %s\n", environment, name)
		return nil
	}}
	list := &cobra.Command{Use: "list", Short: "List saved accounts for the selected environment", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		environment, active, err := accountNames()
		if err != nil {
			return err
		}
		names := []string{active}
		for name := range viper.GetStringMap("accounts." + environment) {
			if !slices.Contains(names, name) {
				names = append(names, name)
			}
		}
		for name := range viper.GetStringMap("profiles") {
			if selectionNamePattern.MatchString(name) && !slices.Contains(names, name) {
				names = append(names, name)
			}
		}
		slices.Sort(names)
		for _, name := range names {
			profile, err := resolveNamedAccount(environment, name)
			if err != nil {
				return err
			}
			if name != active && profile.AccessToken == "" {
				continue
			}
			marker := " "
			if name == active {
				marker = "*"
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%s %s\t%s\tcredentials saved: %t\n", marker, name, environment, profile.AccessToken != "" && profile.Client != "" && profile.UID != "")
		}
		return nil
	}}
	command.AddCommand(show, use, list)
	return command
}
