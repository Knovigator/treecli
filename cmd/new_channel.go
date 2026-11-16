package cmd

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

var newChannelCmd = &cobra.Command{
	Use:   "channel <name>",
	Short: "Create a new channel",
	Long:  "Create a new channel (stream) owned by the current account. The channel is private unless --public is passed: only its members can read and post in it. Post into it with `treecli new post --stream <name-or-id>`.",
	Example: "  treecli new channel \"Agent notes\"\n" +
		"  treecli new channel \"Agent notes\" --description \"Where the agents keep their notes\"\n" +
		"  treecli new channel \"Open floor\" --public",
	Args: cobra.ExactArgs(1),
	RunE: runNewChannel,
}

var channelDescription string
var channelPublic bool
var channelPrivate bool
var channelSpaceID string
var channelWriteID string

type channelCreateOptions struct {
	WriteID     string
	Name        string
	Description string
	Public      *bool
	Private     *bool
	SpaceID     string
}

type createChannelResponse struct {
	Team struct {
		ID     string `json:"id"`
		Name   string `json:"name"`
		Public bool   `json:"public"`
	} `json:"team"`
	Raw json.RawMessage `json:"-"`
}

func init() {
	NewCmd.AddCommand(newChannelCmd)

	newChannelCmd.Flags().StringVar(&channelDescription, "description", "", "Optional description shown on the channel")
	newChannelCmd.Flags().BoolVar(&channelPublic, "public", false, "Make the channel public: anyone in its space can read it")
	newChannelCmd.Flags().BoolVar(&channelPrivate, "private", false, "Make the channel private: only members can read and post (default)")
	newChannelCmd.Flags().StringVar(&channelSpaceID, "space-id", "", "Space ID to create the channel in")
	newChannelCmd.Flags().StringVar(&channelWriteID, "id", "", "UUID for this write; reuse it to retry safely")
	newChannelCmd.Flags().StringVarP(&createOutputFormat, "output", "o", "ascii", "Output format: ascii or json")
	newChannelCmd.Flags().BoolVar(&createJSONOutput, "json", false, "Output JSON instead of human-readable text")
}

func runNewChannel(cmd *cobra.Command, args []string) error {
	name := strings.TrimSpace(args[0])
	if name == "" {
		return usageErrorf("a channel name is required")
	}
	if channelPublic && channelPrivate {
		return usageErrorf("choose --public or --private, not both")
	}

	resolvedOutputFormat := resolveOutputFormat(createOutputFormat, createJSONOutput)
	if resolvedOutputFormat != "ascii" && resolvedOutputFormat != "json" {
		return invalidOutputFormatError(resolvedOutputFormat)
	}

	profile, err := requireAuthenticatedProfile()
	if err != nil {
		return err
	}

	options := channelCreateOptions{
		WriteID:     channelWriteID,
		Name:        name,
		Description: channelDescription,
		SpaceID:     channelSpaceID,
	}
	if cmd.Flags().Changed("public") {
		options.Public = &channelPublic
	}
	if cmd.Flags().Changed("private") {
		options.Private = &channelPrivate
	}

	result, writeID, err := createChannel(profile, options)
	if err != nil {
		return writeErrorForOutput(fmt.Errorf("creating channel: %w", err), resolvedOutputFormat)
	}

	if resolvedOutputFormat == "json" {
		prettyJSON, err := prettyWriteSuccessJSON(result.Raw, writeID)
		if err != nil {
			return fmt.Errorf("formatting JSON: %w", err)
		}
		fmt.Println(prettyJSON)
		return nil
	}

	visibility := "private"
	if result.Team.Public {
		visibility = "public"
	}
	fmt.Printf("Channel created (%s): %s\n", visibility, result.Team.Name)
	fmt.Printf("Channel ID: %s\n", result.Team.ID)
	fmt.Printf("Write ID: %s\n", writeID)
	fmt.Printf("Link: %s\n", channelLink(profile, result.Team.ID))
	return nil
}

// createChannel sends the shared `channel` write. The backend decides the
// default visibility (private) and owns every permission check.
func createChannel(profile profileConfig, options channelCreateOptions) (createChannelResponse, string, error) {
	args := map[string]interface{}{"id": options.WriteID, "name": options.Name}
	optional(args, "description", options.Description)
	optional(args, "space_id", options.SpaceID)
	optionalBool(args, "public", options.Public)
	optionalBool(args, "private", options.Private)
	var raw json.RawMessage
	var out createChannelResponse
	if err := sharedNativeWrite(profile, "channel", args, "", 0, &raw); err != nil {
		return out, stringValue(args["id"]), err
	}
	writeID := stringValue(args["id"])
	if err := json.Unmarshal(raw, &out); err != nil {
		return out, writeID, withWriteID(writeID, err)
	}
	if out.Team.ID == "" {
		return out, writeID, withWriteID(writeID, fmt.Errorf("backend omitted the created channel"))
	}
	out.Raw = raw
	return out, writeID, nil
}

func channelLink(profile profileConfig, channelID string) string {
	linkBase := strings.TrimRight(profile.AppHost, "/")
	if linkBase == "" {
		linkBase = strings.TrimRight(profile.BackendURL, "/")
	}

	return fmt.Sprintf("%s/stream/%s", linkBase, channelID)
}
