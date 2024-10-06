package cmd

import (
	"github.com/spf13/cobra"
)

var NewCmd = &cobra.Command{
	Use:   "new",
	Short: "Create new resources",
	Long:  `Create new resources such as posts or clips.`,
}

func init() {
	NewCmd.AddCommand(newPostCmd)
	NewCmd.AddCommand(newClipCmd)
}
