package main

import (
	"fmt"
	"io"
	"os"

	"github.com/Knovigator/treecli/cmd"
	"github.com/adrg/xdg"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var rootCmd = &cobra.Command{
	Use:           "treecli",
	Short:         "treecli controls Treechat",
	Long:          `A CLI application for interacting with Treechat.`,
	Version:       cmd.CurrentVersion,
	SilenceErrors: true,
	SilenceUsage:  true,
}

const (
	configPath       = "treecli/config.toml"
	legacyConfigPath = "treectl/config.toml"
)

const bashCompletionPrelude = `
if [[ $(type -t _get_comp_words_by_ref 2>/dev/null) != function ]]; then
_get_comp_words_by_ref()
{
    local current_index current_word previous_word

    while [[ $# -gt 0 && $1 == -* ]]; do
        case "$1" in
            -n)
                shift 2
                ;;
            *)
                shift
                ;;
        esac
    done

    current_index=${COMP_CWORD:-0}
    current_word="${COMP_WORDS[current_index]}"
    previous_word=""
    if (( current_index > 0 )); then
        previous_word="${COMP_WORDS[current_index-1]}"
    fi

    while [[ $# -gt 0 ]]; do
        case "$1" in
            cur)
                printf -v "$1" '%s' "$current_word"
                ;;
            prev)
                printf -v "$1" '%s' "$previous_word"
                ;;
            words)
                eval "$1=(\"\${COMP_WORDS[@]}\")"
                ;;
            cword)
                printf -v "$1" '%s' "$current_index"
                ;;
        esac
        shift
    done
}
fi

`

func init() {
	cobra.OnInitialize(initConfig)
	rootCmd.PersistentFlags().StringVar(&cmd.SelectedProfile, "profile", "", "Profile to use (dev, staging, prod, or custom)")
	rootCmd.PersistentFlags().StringVar(&cmd.BackendURLOverride, "backend-url", "", "Override the backend API base URL for this invocation")
	rootCmd.PersistentFlags().StringVar(&cmd.AppHostOverride, "app-host", "", "Override the app host for generated links for this invocation")
	rootCmd.AddCommand(cmd.LoginCmd)
	rootCmd.AddCommand(cmd.GetCmd)
	rootCmd.AddCommand(cmd.ActionCmd)
	rootCmd.AddCommand(cmd.GenerateCmd)
	rootCmd.AddCommand(cmd.NewCmd) // Add the new top-level command
	rootCmd.AddCommand(cmd.ProfileCmd)
	rootCmd.AddCommand(cmd.OnboardCmd)
	rootCmd.AddCommand(cmd.SkillsCmd)
	rootCmd.AddCommand(cmd.UpdateCmd)
	rootCmd.InitDefaultCompletionCmd()
	configureCompletionHelp()
}

func initConfig() {
	configPath, err := resolveConfigPath()
	if err != nil {
		fmt.Println("Error getting config file path:", err)
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
			fmt.Println("Error reading config file:", err)
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
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
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
				originalRunE := completionChild.RunE
				completionChild.RunE = func(cmd *cobra.Command, args []string) error {
					_, err := io.WriteString(cmd.OutOrStdout(), bashCompletionPrelude)
					if err != nil {
						return err
					}

					if originalRunE == nil {
						return nil
					}

					return originalRunE(cmd, args)
				}
			} else {
				completionChild.Example = fmt.Sprintf("  source <(treecli completion %s)", completionChild.Name())
			}
		}
	}
}
