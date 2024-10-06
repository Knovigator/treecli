package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/Knovigator/knovigator/treectl/cmd"
	"github.com/adrg/xdg"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var rootCmd = &cobra.Command{
	Use:   "treectl",
	Short: "treectl is a CLI for managing your tree",
	Long:  `A CLI application for interacting with and managing your tree-related tasks and data.`,
}

func init() {
	cobra.OnInitialize(initConfig)
	rootCmd.AddCommand(cmd.LoginCmd)
	rootCmd.AddCommand(cmd.GetCmd)
	rootCmd.AddCommand(cmd.NewCmd) // Add the new top-level command
}

func initConfig() {
	configPath, err := xdg.ConfigFile("treectl/config.toml")
	if err != nil {
		fmt.Println("Error getting config file path:", err)
		return
	}

	viper.SetConfigFile(configPath)
	viper.SetConfigType("toml")

	// create config file if it doesn't exist
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		err = os.MkdirAll(filepath.Dir(configPath), 0755)
		if err != nil {
			fmt.Println("Error creating config directory:", err)
			return
		}
		_, err = os.Create(configPath)
		if err != nil {
			fmt.Println("Error creating config file:", err)
			return
		}
	}

	if err := viper.ReadInConfig(); err != nil {
		fmt.Println("Error reading config file:", err)
	}
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
