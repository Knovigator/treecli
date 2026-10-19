package main

import (
	"fmt"
	"os"

	"github.com/Knovigator/treecli/cmd"
	"github.com/adrg/xdg"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var rootCmd = &cobra.Command{
	Use:               "treecli",
	Short:             "treecli controls Treechat",
	Long:              `A CLI application for interacting with Treechat.`,
	Version:           cmd.CurrentVersion,
	SilenceErrors:     true,
	SilenceUsage:      true,
	PersistentPreRunE: cmd.PrepareCommand,
}

const (
	configPath       = "treecli/config.toml"
	legacyConfigPath = "treectl/config.toml"
)

const bashCompletionPrelude = cmd.BashCompletionPrelude

func init() {
	cobra.OnInitialize(initConfig)
	rootCmd.PersistentFlags().StringVar(&cmd.SelectedEnvironment, "env", "", "Server environment (default: production; prod, staging, dev, or a custom name)")
	rootCmd.PersistentFlags().StringVar(&cmd.SelectedAccount, "account", "", "Saved account in the selected environment (default: that environment’s active account, or default)")
	rootCmd.PersistentFlags().StringVar(&cmd.SelectedProfile, "profile", "", "Profile to use (default: prod; dev, staging, prod, or custom)")
	rootCmd.PersistentFlags().StringVar(&cmd.BackendURLOverride, "backend-url", "", "Override the backend API base URL for this invocation")
	rootCmd.PersistentFlags().StringVar(&cmd.AppHostOverride, "app-host", "", "Override the app host for generated links for this invocation")
	rootCmd.AddCommand(cmd.LoginCmd)
	rootCmd.AddCommand(cmd.ConnectCmd)
	rootCmd.AddCommand(cmd.SignupCmd)
	rootCmd.AddCommand(cmd.GetCmd)
	rootCmd.AddCommand(cmd.ActionCmd)
	rootCmd.AddCommand(cmd.GenerateCmd)
	rootCmd.AddCommand(cmd.BranchReplyCmd)
	rootCmd.AddCommand(cmd.QuoteReplyCmd)
	rootCmd.AddCommand(cmd.NewCmd) // Add the new top-level command
	rootCmd.AddCommand(cmd.BillingCmd)
	_ = rootCmd.PersistentFlags().MarkDeprecated("profile", "use --env and --account to select the server and identity separately")
	rootCmd.AddCommand(cmd.AccountCmd)
	rootCmd.AddCommand(cmd.WhoamiCmd)
	rootCmd.AddCommand(cmd.ProfileCmd)
	rootCmd.AddCommand(cmd.OnboardCmd)
	rootCmd.AddCommand(cmd.SkillsCmd)
	rootCmd.AddCommand(cmd.McpCmd)
	rootCmd.AddCommand(cmd.UpdateCmd)
	rootCmd.AddCommand(cmd.ToolsCmd)
	rootCmd.AddCommand(cmd.MCPCmd)
	rootCmd.AddCommand(cmd.ReferenceManifestCmd)
	rootCmd.InitDefaultCompletionCmd()
	configureCompletionHelp()
}

func initConfig() {
	configPath, err := resolveConfigPath()
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error getting config file path:", err)
		return
	}

	viper.SetConfigFile(configPath)
	viper.SetConfigType("toml")
	viper.SetEnvPrefix("TREECLI")
	viper.AutomaticEnv()
	_ = viper.BindEnv("active_profile", "TREECLI_ACTIVE_PROFILE", "TREECTL_ACTIVE_PROFILE")

	if err := viper.ReadInConfig(); err != nil {
		_, isConfigMissing := err.(viper.ConfigFileNotFoundError)
		if !isConfigMissing && !os.IsNotExist(err) {
			fmt.Fprintln(os.Stderr, "Error reading config file:", err)
		}
	}
}

func resolveConfigPath() (string, error) {
	nextPath, err := xdg.ConfigFile(configPath)
	if err != nil {
		return "", err
	}
	if _, err := os.Stat(nextPath); err == nil {
		return nextPath, nil
	} else if !os.IsNotExist(err) {
		return "", err
	}

	oldPath, err := xdg.ConfigFile(legacyConfigPath)
	if err != nil {
		return "", err
	}
	if _, err := os.Stat(oldPath); err == nil {
		return oldPath, nil
	} else if !os.IsNotExist(err) {
		return "", err
	}

	return nextPath, nil
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		cmd.PrintError(os.Stderr, err)
		os.Exit(cmd.ExitCode(err))
	}
}

func configureCompletionHelp() {
	completionCmd, _, err := rootCmd.Find([]string{"completion"})
	if err != nil || completionCmd == nil {
		return
	}

	completionCmd.Long = `Generate the autocompletion script for the specified shell.

You can turn completions on immediately in your current shell with:

	if [ -n "${ZSH_VERSION:-}" ]; then autoload -U compinit && compinit; source <(treecli completion zsh); elif command -v complete >/dev/null 2>&1; then source <(treecli completion bash); else echo "Current shell does not support bash completion; use zsh or a bash with progcomp."; fi

Some bash builds, including the one shipped on this machine, do not include programmable completion support. If command -v complete fails, use zsh or a different bash build.

If you want persistent completions, see each shell subcommand's help for install details.`
	completionCmd.Example = "  if [ -n \"${ZSH_VERSION:-}\" ]; then autoload -U compinit && compinit; source <(treecli completion zsh); elif command -v complete >/dev/null 2>&1; then source <(treecli completion bash); else echo \"Current shell does not support bash completion; use zsh or a bash with progcomp.\"; fi\n" +
		"  treecli completion bash\n" +
		"  treecli completion zsh"

	for _, completionChild := range completionCmd.Commands() {
		completionChild.RunE = func(command *cobra.Command, args []string) error {
			plain, err := command.Flags().GetBool("no-descriptions")
			if err != nil {
				return err
			}
			return cmd.RenderCompletion(command.Root(), command.Name(), command.OutOrStdout(), plain)
		}
		switch completionChild.Name() {
		case "bash", "zsh":
			if completionChild.Name() == "bash" {
				completionChild.Long = `Generate the autocompletion script for the bash shell.

treecli's bash completion script is self-contained and does not require the external bash-completion helper library.
It still requires a bash build with programmable completion support. If command -v complete fails, this shell cannot load bash completions.

To load completions in your current shell session:

	if command -v complete >/dev/null 2>&1; then source <(treecli completion bash); else echo "This bash build does not support programmable completion."; fi

To load completions for every new session, execute once:

#### Linux:

	treecli completion bash > /etc/bash_completion.d/treecli

#### macOS:

	treecli completion bash > $(brew --prefix)/etc/bash_completion.d/treecli

You will need to start a new shell for this setup to take effect.`
				completionChild.Example = "  if command -v complete >/dev/null 2>&1; then source <(treecli completion bash); else echo \"This bash build does not support programmable completion.\"; fi"

			} else {
				completionChild.Example = fmt.Sprintf("  source <(treecli completion %s)", completionChild.Name())
			}
		}
	}
}
