package cmd

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

var ChannelCmd = &cobra.Command{
	Use:   "channel",
	Short: "Manage channels you own",
	Long:  "Manage channels (streams) you own. Create one with `treecli new channel`.",
}

var channelInviteCmd = &cobra.Command{
	Use:   "invite <channel> <username> [username...]",
	Short: "Invite people to a channel you own",
	Long:  "Invite existing accounts, by username, to a channel you own. Each person gets a pending invite and joins only when they accept it. Up to 20 people per call; the account's daily invite limit applies.",
	Example: "  treecli channel invite \"Agent notes\" alice\n" +
		"  treecli channel invite \"Agent notes\" alice @bob\n" +
		"  treecli channel invite 2c54a2de-4a8a-47a6-a9ea-7eda96a0ebf7 alice",
	Args: cobra.MinimumNArgs(2),
	RunE: runChannelInvite,
}

var channelInviteWriteID string

type channelMember struct {
	Name string `json:"name"`
}

type channelInviteResponse struct {
	Team struct {
		ID           string          `json:"id"`
		Name         string          `json:"name"`
		Users        []channelMember `json:"users"`
		PendingUsers []channelMember `json:"pending_users"`
	} `json:"team"`
	Raw json.RawMessage `json:"-"`
}

func init() {
	ChannelCmd.AddCommand(channelInviteCmd)

	channelInviteCmd.Flags().StringVar(&channelInviteWriteID, "id", "", "UUID for this write; reuse it to retry safely")
	channelInviteCmd.Flags().StringVarP(&createOutputFormat, "output", "o", "ascii", "Output format: ascii or json")
	channelInviteCmd.Flags().BoolVar(&createJSONOutput, "json", false, "Output JSON instead of human-readable text")
}

func runChannelInvite(cmd *cobra.Command, args []string) error {
	channel := strings.TrimSpace(args[0])
	if channel == "" {
		return usageErrorf("a channel name or ID is required")
	}
	usernames := cleanUsernames(args[1:])
	if len(usernames) == 0 {
		return usageErrorf("at least one username is required")
	}

	resolvedOutputFormat := resolveOutputFormat(createOutputFormat, createJSONOutput)
	if resolvedOutputFormat != "ascii" && resolvedOutputFormat != "json" {
		return invalidOutputFormatError(resolvedOutputFormat)
	}

	profile, err := requireAuthenticatedProfile()
	if err != nil {
		return err
	}

	result, writeID, err := inviteToChannel(profile, channelInviteWriteID, channel, usernames)
	if err != nil {
		return writeErrorForOutput(fmt.Errorf("inviting to channel: %w", err), resolvedOutputFormat)
	}

	if resolvedOutputFormat == "json" {
		prettyJSON, err := prettyWriteSuccessJSON(result.Raw, writeID)
		if err != nil {
			return fmt.Errorf("formatting JSON: %w", err)
		}
		fmt.Println(prettyJSON)
		return nil
	}

	fmt.Printf("Channel: %s (%s)\n", result.Team.Name, result.Team.ID)
	for _, username := range usernames {
		fmt.Printf("  @%s: %s\n", username, inviteOutcome(result, username))
	}
	fmt.Printf("Write ID: %s\n", writeID)
	return nil
}

// cleanUsernames trims, drops a leading @ and removes blanks and repeats.
func cleanUsernames(values []string) []string {
	seen := map[string]bool{}
	usernames := []string{}
	for _, value := range values {
		username := strings.TrimPrefix(strings.TrimSpace(value), "@")
		key := strings.ToLower(username)
		if username == "" || seen[key] {
			continue
		}
		seen[key] = true
		usernames = append(usernames, username)
	}
	return usernames
}

// inviteOutcome reads what happened to one username from the channel the
// backend returned. Someone who muted the inviter is skipped without an invite.
func inviteOutcome(result channelInviteResponse, username string) string {
	for _, member := range result.Team.Users {
		if strings.EqualFold(member.Name, username) {
			return "already a member"
		}
	}
	for _, pending := range result.Team.PendingUsers {
		if strings.EqualFold(pending.Name, username) {
			return "invited, waiting for them to accept"
		}
	}
	return "not invited"
}

// inviteToChannel sends the shared `channel_invite` write. The backend checks
// that the caller owns the channel and that every username is a real account.
func inviteToChannel(profile profileConfig, id, channel string, usernames []string) (channelInviteResponse, string, error) {
	names := make([]interface{}, len(usernames))
	for i, username := range usernames {
		names[i] = username
	}
	args := map[string]interface{}{"id": id, "channel": channel, "usernames": names}
	var raw json.RawMessage
	var out channelInviteResponse
	if err := sharedNativeWrite(profile, "channel_invite", args, "", 0, &raw); err != nil {
		return out, stringValue(args["id"]), err
	}
	writeID := stringValue(args["id"])
	if err := json.Unmarshal(raw, &out); err != nil {
		return out, writeID, withWriteID(writeID, err)
	}
	if out.Team.ID == "" {
		return out, writeID, withWriteID(writeID, fmt.Errorf("backend omitted the channel"))
	}
	out.Raw = raw
	return out, writeID, nil
}
