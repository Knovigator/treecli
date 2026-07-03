package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/Knovigator/treectl/api"
	"github.com/spf13/cobra"
)

var generateActionsJSON bool
var generateActionsDirectOnly bool

type generationActionRow struct {
	Action               string   `json:"action"`
	Name                 string   `json:"name,omitempty"`
	Description          string   `json:"description,omitempty"`
	Provider             string   `json:"provider,omitempty"`
	Kind                 string   `json:"kind,omitempty"`
	DirectGeneration     bool     `json:"direct_generation"`
	Async                bool     `json:"async"`
	AcceptsReference     bool     `json:"accepts_reference"`
	SupportsInstrumental bool     `json:"supports_instrumental"`
	DurationMin          int      `json:"duration_min,omitempty"`
	DurationMax          int      `json:"duration_max,omitempty"`
	Inputs               []string `json:"inputs,omitempty"`
}

// generateActionsCmd lists active AI actions and marks which ones the direct generation endpoint supports.
var generateActionsCmd = &cobra.Command{
	Use:     "actions",
	Aliases: []string{"tags"},
	Short:   "List AI actions available from the active backend profile",
	Long: "List active AI actions from the backend model catalog and mark which ones support " +
		"direct post-less generation through `treectl generate`.\n\n" +
		"`treectl generate tags` is kept as a compatibility alias for older scripts.",
	Args: cobra.NoArgs,
	RunE: runGenerateActions,
}

func init() {
	generateActionsCmd.Flags().BoolVar(&generateActionsJSON, "json", false, "Print the actions as JSON")
	generateActionsCmd.Flags().BoolVar(&generateActionsDirectOnly, "direct-only", false, "Only list AI actions supported by post-less generation")
}

func runGenerateActions(cmd *cobra.Command, args []string) error {
	profile, err := requireAuthenticatedProfile()
	if err != nil {
		return err
	}

	models, err := fetchVisibleActionModels(profile)
	if err != nil {
		return fmt.Errorf("loading AI actions: %w", err)
	}

	directTags, err := api.ListGenerationTags(profile.BackendURL, profile.AccessToken, profile.Client, profile.UID)
	if err != nil {
		return fmt.Errorf("loading direct generation support: %w", err)
	}

	rows := generationActionRows(models, directTags, generateActionsDirectOnly)

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

	w := tabwriter.NewWriter(os.Stdout, 0, 2, 2, ' ', 0)
	fmt.Fprintln(w, "ACTION\tNAME\tPROVIDER\tKIND\tDIRECT\tASYNC\tREF\tINSTR\tDURATION\tINPUTS")
	for _, row := range rows {
		duration := "-"
		if row.DurationMax > 0 {
			duration = fmt.Sprintf("%d-%ds", row.DurationMin, row.DurationMax)
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
			row.Action, dash(row.Name), dash(row.Provider), dash(row.Kind),
			yesno(row.DirectGeneration), yesno(row.Async), yesno(row.AcceptsReference),
			yesno(row.SupportsInstrumental), duration, dash(strings.Join(row.Inputs, ",")))
	}
	return w.Flush()
}

func generationActionRows(models []api.AIModelRef, directTags []api.TagInfo, directOnly bool) []generationActionRow {
	directByAction := map[string]api.TagInfo{}
	for _, directTag := range directTags {
		if shouldHideDirectGenerationAction(directTag) {
			continue
		}
		action := normalizedActionName(directTag.Tag)
		if action == "" {
			continue
		}
		directByAction[action] = directTag
	}

	rows := []generationActionRow{}
	seen := map[string]bool{}
	for _, model := range models {
		if shouldHideActionModel(model) {
			continue
		}

		action := normalizedActionName(model.ActionTagName)
		if action == "" {
			continue
		}

		directTag, direct := directByAction[action]
		if directOnly && !direct {
			continue
		}

		rows = append(rows, generationActionRowFromModel(model, directTag, direct))
		seen[action] = true
	}

	for _, directTag := range directTags {
		if shouldHideDirectGenerationAction(directTag) {
			continue
		}
		action := normalizedActionName(directTag.Tag)
		if action == "" || seen[action] {
			continue
		}
		rows = append(rows, generationActionRowFromDirectTag(directTag))
		seen[action] = true
	}

	sort.Slice(rows, func(left int, right int) bool {
		leftKind := strings.ToLower(rows[left].Kind)
		rightKind := strings.ToLower(rows[right].Kind)
		if leftKind != rightKind {
			return leftKind < rightKind
		}
		return strings.ToLower(rows[left].Action) < strings.ToLower(rows[right].Action)
	})

	return rows
}

func generationActionRowFromModel(model api.AIModelRef, directTag api.TagInfo, direct bool) generationActionRow {
	name := firstNonBlank(model.DisplayName, model.HumanName, model.Name)
	description := firstNonBlank(model.DescriptionShort, model.Description)
	provider := firstNonBlank(directTag.Provider, model.Provider)
	kind := firstNonBlank(directTag.Kind, model.ModelType)

	row := generationActionRow{
		Action:           strings.TrimSpace(model.ActionTagName),
		Name:             name,
		Description:      description,
		Provider:         provider,
		Kind:             kind,
		DirectGeneration: direct,
	}
	if direct {
		row.Async = directTag.Async
		row.AcceptsReference = directTag.AcceptsReference
		row.SupportsInstrumental = directTag.SupportsInstrumental
		row.DurationMin = directTag.DurationMin
		row.DurationMax = directTag.DurationMax
		row.Inputs = directTag.Inputs
	}
	return row
}

func generationActionRowFromDirectTag(directTag api.TagInfo) generationActionRow {
	return generationActionRow{
		Action:               strings.TrimSpace(directTag.Tag),
		Provider:             strings.TrimSpace(directTag.Provider),
		Kind:                 strings.TrimSpace(directTag.Kind),
		DirectGeneration:     true,
		Async:                directTag.Async,
		AcceptsReference:     directTag.AcceptsReference,
		SupportsInstrumental: directTag.SupportsInstrumental,
		DurationMin:          directTag.DurationMin,
		DurationMax:          directTag.DurationMax,
		Inputs:               directTag.Inputs,
	}
}

func shouldHideDirectGenerationAction(directTag api.TagInfo) bool {
	if strings.EqualFold(strings.TrimSpace(directTag.Provider), "openclaw") {
		return true
	}
	return strings.HasPrefix(normalizedActionName(directTag.Tag), "openclaw")
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
