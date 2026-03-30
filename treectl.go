package main

import (
	"fmt"
	"os"

	"github.com/Knovigator/knovigator/treectl/cmd"
	"github.com/adrg/xdg"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var rootCmd = &cobra.Command{
	Use:   "treectl",
	Short: "treectl controls Treechat",
	Long:  `A CLI application for interacting with Treechat.`,
}

func init() {
	cobra.OnInitialize(initConfig)
	rootCmd.PersistentFlags().StringVar(&cmd.SelectedProfile, "profile", "", "Profile to use (dev, staging, prod, or custom)")
	rootCmd.PersistentFlags().StringVar(&cmd.BackendURLOverride, "backend-url", "", "Override the backend API base URL for this invocation")
	rootCmd.PersistentFlags().StringVar(&cmd.AppHostOverride, "app-host", "", "Override the app host for generated links for this invocation")
	rootCmd.AddCommand(cmd.LoginCmd)
	rootCmd.AddCommand(cmd.GetCmd)
	rootCmd.AddCommand(cmd.ActionCmd)
	rootCmd.AddCommand(cmd.NewCmd) // Add the new top-level command
	rootCmd.AddCommand(cmd.ProfileCmd)
	rootCmd.AddCommand(cmd.SkillsCmd)
	rootCmd.InitDefaultCompletionCmd()
}

func initConfig() {
	configPath, err := xdg.ConfigFile("treectl/config.toml")
	if err != nil {
		fmt.Println("Error getting config file path:", err)
		return
	}

	viper.SetConfigFile(configPath)
	viper.SetConfigType("toml")
	viper.SetEnvPrefix("TREECTL")
	viper.AutomaticEnv()

	if err := viper.ReadInConfig(); err != nil {
		_, isConfigMissing := err.(viper.ConfigFileNotFoundError)
		if !isConfigMissing && !os.IsNotExist(err) {
			fmt.Println("Error reading config file:", err)
		}
	}
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
