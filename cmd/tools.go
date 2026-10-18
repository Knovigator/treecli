package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/Knovigator/treecli/api"
	"github.com/Knovigator/treecli/mcpapi"
	"github.com/spf13/cobra"
)

func selectedToolsClient() (*api.Client, error) {
	profile, err := requireAuthenticatedProfile()
	if err != nil {
		return nil, err
	}
	return api.NewTokenClient(profile.BackendURL, profile.AccessToken, profile.Client, profile.UID)
}

var ToolsCmd = newToolsCommand()
var MCPCmd = newMCPCommand()

func newToolsCommand() *cobra.Command {
	root := &cobra.Command{Use: "tools", Short: "Use the shared Treechat tool catalog"}
	root.AddCommand(&cobra.Command{Use: "list", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		client, err := selectedToolsClient()
		if err != nil {
			return err
		}
		result, err := client.ListTools(cmd.Context())
		if err != nil {
			return err
		}
		return json.NewEncoder(cmd.OutOrStdout()).Encode(result)
	}})
	arguments := "{}"
	call := &cobra.Command{Use: "call NAME", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		if len(arguments) > mcpapi.MaxMessageBytes {
			return fmt.Errorf("arguments exceed 72 MiB")
		}
		var values map[string]interface{}
		if err := json.Unmarshal([]byte(arguments), &values); err != nil || values == nil {
			return usageErrorf("arguments must be a JSON object")
		}
		client, err := selectedToolsClient()
		if err != nil {
			return err
		}
		result, err := client.CallTool(cmd.Context(), args[0], values)
		if err != nil {
			return err
		}
		if err := json.NewEncoder(cmd.OutOrStdout()).Encode(result); err != nil {
			return err
		}
		if result.IsError {
			return fmt.Errorf("Treechat tool reported an error")
		}
		return nil
	}}
	call.Flags().StringVar(&arguments, "arguments", "{}", "Tool arguments as a JSON object")
	root.AddCommand(call)
	return root
}
func newMCPCommand() *cobra.Command {
	root := &cobra.Command{Use: "mcp", Short: "Serve the shared Treechat API tools"}
	root.AddCommand(&cobra.Command{Use: "serve", Short: "Serve MCP over stdio for the selected account", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		client, err := selectedToolsClient()
		if err != nil {
			return err
		}
		return mcpapi.Serve(cmd.Context(), cmd.InOrStdin(), cmd.OutOrStdout(), client, CurrentVersion)
	}})
	return root
}
