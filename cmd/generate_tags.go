package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"
)

var generateActionsJSON bool
var generateActionsDirectOnly bool
var generateActionsVerbose bool
var generateDescribeJSON bool

type generationActionRow struct {
	PromptOptional       bool          `json:"prompt_optional"`
	Action               string        `json:"action"`
	Name                 string        `json:"name,omitempty"`
	Description          string        `json:"description,omitempty"`
	Provider             string        `json:"provider,omitempty"`
	Kind                 string        `json:"kind,omitempty"`
	DirectGeneration     bool          `json:"direct_generation"`
	Async                bool          `json:"async"`
	AcceptsReference     bool          `json:"accepts_reference"`
	RequiresReference    bool          `json:"requires_reference"`
	ReferenceKinds       []string      `json:"reference_kinds,omitempty"`
	SupportsInstrumental bool          `json:"supports_instrumental"`
	DurationMin          int           `json:"duration_min,omitempty"`
	DurationMax          int           `json:"duration_max,omitempty"`
	Inputs               []string      `json:"inputs,omitempty"`
	Settings             []settingHelp `json:"settings,omitempty"`
	Examples             []string      `json:"examples,omitempty"`
	Notes                []string      `json:"notes,omitempty"`
	Aliases              []string      `json:"aliases,omitempty"`
}

type settingHelp struct {
	Name        string `json:"name"`
	Type        string `json:"type,omitempty"`
	How         string `json:"how,omitempty"`
	Description string `json:"description"`
	Example     string `json:"example,omitempty"`
}

// generateActionsCmd lists active AI actions and marks which ones the direct generation endpoint supports.
var generateActionsCmd = &cobra.Command{
	Use:   "actions",
	Short: "List AI actions available from the active backend profile",
	Long: "List active AI actions from the backend model catalog and mark which ones support " +
		"direct post-less generation through `treecli generate`.",
	Args: cobra.NoArgs,
	RunE: runGenerateActions,
}

var generateTagsCompatCmd = &cobra.Command{
	Use:    "tags",
	Hidden: true,
	Args:   cobra.NoArgs,
	RunE:   runGenerateActions,
}

var generateDescribeCmd = &cobra.Command{
	Use:   "describe <ai-action>",
	Short: "Show detailed help for one AI action",
	Long: "Show detailed help for one AI action, including model description, direct-generation " +
		"support, accepted settings, and example commands for humans and agents.",
	Example: "  treecli generate describe flux2\n" +
		"  treecli generate describe suno --json\n" +
		"  treecli generate describe veo3",
	Args:              cobra.ExactArgs(1),
	RunE:              runGenerateDescribe,
	ValidArgsFunction: completeGenerateDescribeArgs,
}

func init() {
	configureGenerateActionsFlags(generateActionsCmd)
	configureGenerateActionsFlags(generateTagsCompatCmd)
	generateDescribeCmd.Flags().BoolVar(&generateDescribeJSON, "json", false, "Print the action detail as JSON")
}

func configureGenerateActionsFlags(command *cobra.Command) {
	command.Flags().BoolVar(&generateActionsJSON, "json", false, "Print the actions as JSON")
	command.Flags().BoolVar(&generateActionsDirectOnly, "direct-only", false, "Only list AI actions supported by post-less generation")
	command.Flags().BoolVar(&generateActionsVerbose, "verbose", false, "Print descriptions, settings, examples, and notes for each action")
}

func runGenerateActions(cmd *cobra.Command, args []string) error {
	profile, err := requireAuthenticatedProfile()
	if err != nil {
		return err
	}

	var catalog struct {
		Actions []generationActionRow `json:"actions"`
	}
	if err := sharedCall(profile, "treechat_read", map[string]interface{}{"operation": "generation_actions", "arguments": map[string]interface{}{"direct_only": generateActionsDirectOnly}}, &catalog); err != nil {
		return err
	}
	rows := catalog.Actions

	if generateActionsJSON {
		encoded, err := json.MarshalIndent(rows, "", "  ")
		if err != nil {
			return fmt.Errorf("formatting JSON: %w", err)
		}
		fmt.Println(string(encoded))
		return nil
	}

	if len(rows) == 0 {
		if generateActionsDirectOnly {
			fmt.Println("No directly generatable AI actions available.")
		} else {
			fmt.Println("No AI actions available.")
		}
		return nil
	}

	if generateActionsVerbose {
		return printGenerationActionDetails(rows)
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 2, 2, ' ', 0)
	fmt.Fprintln(w, "ACTION\tNAME\tPROVIDER\tKIND\tDIRECT\tASYNC\tREF\tINSTR\tDURATION\tINPUTS")
	for _, row := range rows {
		duration := "-"
		if row.DurationMax > 0 {
			duration = fmt.Sprintf("%d-%ds", row.DurationMin, row.DurationMax)
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
			row.Action, dash(row.Name), dash(row.Provider), dash(row.Kind),
			yesno(row.DirectGeneration), yesno(row.Async), referenceSummary(row),
			yesno(row.SupportsInstrumental), duration, dash(strings.Join(row.Inputs, ",")))
	}
	return w.Flush()
}

func runGenerateDescribe(cmd *cobra.Command, args []string) error {
	profile, err := requireAuthenticatedProfile()
	if err != nil {
		return err
	}

	var row generationActionRow
	if err := sharedCall(profile, "treechat_read", map[string]interface{}{"operation": "describe_action", "arguments": map[string]interface{}{"action": args[0]}}, &row); err != nil {
		return err
	}

	if generateDescribeJSON {
		encoded, err := json.MarshalIndent(row, "", "  ")
		if err != nil {
			return fmt.Errorf("formatting JSON: %w", err)
		}
		fmt.Println(string(encoded))
		return nil
	}

	return printGenerationActionDetail(row)
}

func completeGenerateDescribeArgs(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if len(args) > 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	return completeGenerationActionNames(toComplete, false)
}

func completeGenerationActionNames(toComplete string, directOnly bool) ([]string, cobra.ShellCompDirective) {
	profile, err := requireAuthenticatedProfile()
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	rows, err := fetchActionCatalog(profile, directOnly)
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	return completeGenerationActionRowNames(rows, toComplete), cobra.ShellCompDirectiveNoFileComp
}

func fetchActionCatalog(profile profileConfig, directOnly bool) ([]generationActionRow, error) {
	var catalog struct {
		Actions []generationActionRow `json:"actions"`
	}
	err := sharedCall(profile, "treechat_read", map[string]interface{}{"operation": "generation_actions", "arguments": map[string]interface{}{"direct_only": directOnly}}, &catalog)
	return catalog.Actions, err
}

func completeGenerationActionRowNames(rows []generationActionRow, toComplete string) []string {
	prefix := normalizedActionName(toComplete)
	bang := strings.HasPrefix(strings.TrimSpace(toComplete), "!")
	completions := []string{}
	seen := map[string]bool{}
	for _, row := range rows {
		for _, action := range append([]string{row.Action}, row.Aliases...) {
			candidate := strings.TrimPrefix(strings.TrimSpace(action), "!")
			key := normalizedActionName(candidate)
			if key == "" || seen[key] || !strings.HasPrefix(key, prefix) {
				continue
			}
			seen[key] = true
			if bang {
				candidate = "!" + candidate
			}
			completions = append(completions, candidate)
		}
	}
	sort.Strings(completions)
	return completions
}

func referenceSummary(row generationActionRow) string {
	if !row.AcceptsReference && !row.RequiresReference {
		return "no"
	}
	label := "yes"
	if row.RequiresReference {
		label = "required"
	}
	if len(row.ReferenceKinds) > 0 {
		label += ":" + strings.Join(row.ReferenceKinds, "|")
	}
	return label
}

func printGenerationActionDetails(rows []generationActionRow) error {
	for index, row := range rows {
		if index > 0 {
			fmt.Println()
		}
		if err := printGenerationActionDetail(row); err != nil {
			return err
		}
	}
	return nil
}

func printGenerationActionDetail(row generationActionRow) error {
	fmt.Printf("AI action: %s\n", row.Action)
	if row.Name != "" {
		fmt.Printf("Name: %s\n", row.Name)
	}
	if row.Description != "" {
		fmt.Printf("Description: %s\n", row.Description)
	}
	fmt.Printf("Provider: %s\n", dash(row.Provider))
	fmt.Printf("Kind: %s\n", dash(row.Kind))
	fmt.Printf("Direct generation: %s\n", yesno(row.DirectGeneration))
	if row.DirectGeneration {
		fmt.Printf("Async: %s\n", yesno(row.Async))
		fmt.Printf("Accepts reference: %s\n", yesno(row.AcceptsReference))
		fmt.Printf("Requires reference: %s\n", yesno(row.RequiresReference))
		if kinds := row.ReferenceKinds; len(kinds) > 0 {
			fmt.Printf("Reference kinds: %s\n", strings.Join(kinds, ", "))
		}
		fmt.Printf("Supports instrumental: %s\n", yesno(row.SupportsInstrumental))
		if row.DurationMax > 0 {
			fmt.Printf("Duration: %d-%ds\n", row.DurationMin, row.DurationMax)
		}
	}
	if len(row.Settings) > 0 {
		fmt.Println("Inputs and settings:")
		for _, setting := range row.Settings {
			line := fmt.Sprintf("  %s", setting.Name)
			if setting.Type != "" {
				line += fmt.Sprintf(" (%s)", setting.Type)
			}
			if setting.How != "" {
				line += fmt.Sprintf(" via %s", setting.How)
			}
			if setting.Description != "" {
				line += fmt.Sprintf(" - %s", setting.Description)
			}
			fmt.Println(line)
			if setting.Example != "" {
				fmt.Printf("    Example: %s\n", setting.Example)
			}
		}
	}
	if len(row.Examples) > 0 {
		fmt.Println("Examples:")
		for _, example := range row.Examples {
			fmt.Printf("  %s\n", example)
		}
	}
	if len(row.Notes) > 0 {
		fmt.Println("Notes:")
		for _, note := range row.Notes {
			fmt.Printf("  %s\n", note)
		}
	}
	return nil
}

func normalizedActionName(action string) string {
	return strings.ToLower(strings.TrimSpace(strings.TrimPrefix(action, "!")))
}

func firstNonBlank(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func yesno(b bool) string {
	if b {
		return "yes"
	}
	return "-"
}

func dash(s string) string {
	if strings.TrimSpace(s) == "" {
		return "-"
	}
	return s
}
