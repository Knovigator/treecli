package cmd

import (
	"github.com/spf13/cobra"
)

var GetCmd = &cobra.Command{
	Use:   "get",
	Short: "Get information about various resources",
	Long:  `Fetch and display information about threads, answers, or other resources.`,
}

func init() {
	GetCmd.AddCommand(getThreadCmd)
	GetCmd.AddCommand(getMessagesCmd)
}
