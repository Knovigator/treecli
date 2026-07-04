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
var generateActionsVerbose bool
var generateDescribeJSON bool

type generationActionRow struct {
	Action               string            `json:"action"`
	Name                 string            `json:"name,omitempty"`
	Description          string            `json:"description,omitempty"`
	Provider             string            `json:"provider,omitempty"`
	Kind                 string            `json:"kind,omitempty"`
	DirectGeneration     bool              `json:"direct_generation"`
	Async                bool              `json:"async"`
	AcceptsReference     bool              `json:"accepts_reference"`
	SupportsInstrumental bool              `json:"supports_instrumental"`
	DurationMin          int               `json:"duration_min,omitempty"`
	DurationMax          int               `json:"duration_max,omitempty"`
	Inputs               []string          `json:"inputs,omitempty"`
	Settings             []settingHelp     `json:"settings,omitempty"`
	Examples             []string          `json:"examples,omitempty"`
	Notes                []string          `json:"notes,omitempty"`
	BackendSettings      []api.SettingInfo `json:"-"`
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
	Use:     "actions",
	Aliases: []string{"tags"},
	Short:   "List AI actions available from the active backend profile",
	Long: "List active AI actions from the backend model catalog and mark which ones support " +
		"direct post-less generation through `treectl generate`.\n\n" +
		"`treectl generate tags` is kept as a compatibility alias for older scripts.",
	Args: cobra.NoArgs,
	RunE: runGenerateActions,
}

var generateDescribeCmd = &cobra.Command{
	Use:   "describe <ai-action>",
	Short: "Show detailed help for one AI action",
	Long: "Show detailed help for one AI action, including model description, direct-generation " +
		"support, accepted settings, and example commands for humans and agents.",
	Example: "  treectl generate describe flux2\n" +
		"  treectl generate describe suno --json\n" +
		"  treectl generate describe veo3",
	Args:              cobra.ExactArgs(1),
	RunE:              runGenerateDescribe,
	ValidArgsFunction: completeGenerateDescribeArgs,
}

func init() {
	generateActionsCmd.Flags().BoolVar(&generateActionsJSON, "json", false, "Print the actions as JSON")
	generateActionsCmd.Flags().BoolVar(&generateActionsDirectOnly, "direct-only", false, "Only list AI actions supported by post-less generation")
	generateActionsCmd.Flags().BoolVar(&generateActionsVerbose, "verbose", false, "Print descriptions, settings, examples, and notes for each action")
	generateDescribeCmd.Flags().BoolVar(&generateDescribeJSON, "json", false, "Print the action detail as JSON")
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
			yesno(row.DirectGeneration), yesno(row.Async), yesno(row.AcceptsReference),
			yesno(row.SupportsInstrumental), duration, dash(strings.Join(row.Inputs, ",")))
	}
	return w.Flush()
}

func runGenerateDescribe(cmd *cobra.Command, args []string) error {
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

	row, ok := findGenerationActionRow(generationActionRows(models, directTags, false), args[0])
	if !ok {
		return fmt.Errorf("unknown AI action %q; run `treectl generate actions` to inspect available actions", args[0])
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

		rows = append(rows, enrichGenerationActionRow(generationActionRowFromModel(model, directTag, direct)))
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
		rows = append(rows, enrichGenerationActionRow(generationActionRowFromDirectTag(directTag)))
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

func completeGenerationActionNames(toComplete string, directOnly bool) ([]string, cobra.ShellCompDirective) {
	profile, err := requireAuthenticatedProfile()
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	models, err := fetchVisibleActionModels(profile)
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	directTags, err := api.ListGenerationTags(profile.BackendURL, profile.AccessToken, profile.Client, profile.UID)
	if err != nil {
		if directOnly {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		return completeGenerationActionRowNames(actionRowsFromModels(models), toComplete), cobra.ShellCompDirectiveNoFileComp
	}

	rows := generationActionRows(models, directTags, directOnly)
	return completeGenerationActionRowNames(rows, toComplete), cobra.ShellCompDirectiveNoFileComp
}

func actionRowsFromModels(models []api.AIModelRef) []generationActionRow {
	rows := make([]generationActionRow, 0, len(models))
	for _, model := range models {
		if shouldHideActionModel(model) {
			continue
		}
		action := strings.TrimSpace(model.ActionTagName)
		if action == "" {
			continue
		}
		rows = append(rows, generationActionRow{Action: action})
	}
	return rows
}

func completeGenerationActionRowNames(rows []generationActionRow, toComplete string) []string {
	normalizedPrefix := normalizedActionName(toComplete)
	wantBangPrefix := strings.HasPrefix(strings.TrimSpace(toComplete), "!")
	completions := []string{}
	seenActions := map[string]bool{}

	for _, row := range rows {
		action := strings.TrimPrefix(strings.TrimSpace(row.Action), "!")
		normalizedAction := normalizedActionName(action)
		if normalizedAction == "" {
			continue
		}
		if normalizedPrefix != "" && !strings.HasPrefix(normalizedAction, normalizedPrefix) {
			continue
		}
		if seenActions[normalizedAction] {
			continue
		}
		seenActions[normalizedAction] = true
		if wantBangPrefix {
			action = "!" + action
		}
		completions = append(completions, action)
	}

	sort.Strings(completions)
	return completions
}

func findGenerationActionRow(rows []generationActionRow, action string) (generationActionRow, bool) {
	normalized := normalizedActionName(action)
	for _, row := range rows {
		if normalizedActionName(row.Action) == normalized {
			return row, true
		}
	}
	return generationActionRow{}, false
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
		row.BackendSettings = directTag.Settings
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
		BackendSettings:      directTag.Settings,
	}
}

func shouldHideDirectGenerationAction(directTag api.TagInfo) bool {
	if strings.EqualFold(strings.TrimSpace(directTag.Provider), "openclaw") {
		return true
	}
	return strings.HasPrefix(normalizedActionName(directTag.Tag), "openclaw")
}

func enrichGenerationActionRow(row generationActionRow) generationActionRow {
	if row.DirectGeneration {
		row.Settings = generationSettingsFor(row)
		row.Examples = generationExamplesFor(row)
		row.Notes = directGenerationNotesFor(row)
		return row
	}

	row.Examples = []string{fmt.Sprintf("treectl action %s \"prompt\"", row.Action)}
	row.Notes = []string{
		"This AI action is available for post-backed Treechat action workflows.",
		"It is not currently advertised by the direct post-less generation endpoint, so use `treectl action` instead of `treectl generate`.",
	}
	return row
}

func generationSettingsFor(row generationActionRow) []settingHelp {
	settings := []settingHelp{
		{
			Name:        "prompt",
			Type:        "string",
			How:         "positional argument",
			Description: "Primary generation prompt. Pass it after the action name.",
			Example:     fmt.Sprintf("treectl generate %s \"a cinematic mountain sunrise\" --out output.%s", row.Action, defaultOutputExtension(row.Kind)),
		},
	}

	if row.DurationMax > 0 {
		settings = append(settings, settingHelp{
			Name:        "duration_seconds",
			Type:        "integer",
			How:         "--duration <seconds> or --input duration_seconds=<seconds>",
			Description: fmt.Sprintf("Requested duration in seconds. The backend clamps to %d-%d seconds.", row.DurationMin, row.DurationMax),
			Example:     "--duration 22",
		})
	}

	if row.SupportsInstrumental {
		settings = append(settings, settingHelp{
			Name:        "instrumental",
			Type:        "boolean",
			How:         "--instrumental or --input instrumental=true",
			Description: "For music actions, request an instrumental result without vocals.",
			Example:     "--instrumental",
		})
	}

	if row.AcceptsReference {
		settings = append(settings, settingHelp{
			Name:        "reference_url",
			Type:        "url",
			How:         "--reference run:<id>, --reference https://..., or --reference @path",
			Description: "Reference media used to steer or chain a generation. Local files are uploaded first.",
			Example:     "--reference run:abc123",
		})
	}

	settings = append(settings, knownSettingsFor(row)...)
	settings = append(settings, backendSettingsFor(row)...)

	seen := map[string]bool{}
	filtered := make([]settingHelp, 0, len(settings)+len(row.Inputs))
	for _, setting := range settings {
		key := normalizedActionName(setting.Name)
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		filtered = append(filtered, setting)
	}
	for _, input := range row.Inputs {
		key := normalizedActionName(input)
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		filtered = append(filtered, settingHelp{
			Name:        strings.TrimSpace(input),
			How:         fmt.Sprintf("--input %s=<value>", strings.TrimSpace(input)),
			Description: "Backend-advertised direct generation setting. Values are parsed as JSON when possible.",
		})
	}
	return filtered
}

func backendSettingsFor(row generationActionRow) []settingHelp {
	settings := make([]settingHelp, 0, len(row.BackendSettings))
	for _, backendSetting := range row.BackendSettings {
		name := strings.TrimSpace(backendSetting.Name)
		if name == "" {
			continue
		}
		help := settingHelp{
			Name:        name,
			Type:        backendSetting.Type,
			How:         fmt.Sprintf("--input %s=<value>", name),
			Description: backendSetting.Description,
		}
		if name == "duration_seconds" {
			help.How = "--duration <seconds> or --input duration_seconds=<seconds>"
		}
		if name == "reference_url" {
			help.Type = firstNonBlank(help.Type, "url")
			help.How = "--reference run:<id>, --reference https://..., or --reference @path"
		}
		if name == "reference_content_type" || name == "reference_kind" {
			help.How = "usually inferred by treectl; may be passed with --input"
		}
		settings = append(settings, help)
	}
	return settings
}

func knownSettingsFor(row generationActionRow) []settingHelp {
	switch normalizedActionName(row.Action) {
	case "flux":
		return []settingHelp{
			{
				Name:        "safety_tolerance",
				Type:        "integer",
				How:         "--input safety_tolerance=<1-6>",
				Description: "Replicate Flux safety tolerance passed through to the provider.",
				Example:     "--input safety_tolerance=5",
			},
		}
	case "flux2":
		return []settingHelp{
			{
				Name:        "aspect_ratio",
				Type:        "string",
				How:         "--input aspect_ratio=<ratio>",
				Description: "Output aspect ratio.",
				Example:     "--input aspect_ratio=3:1",
			},
			{
				Name:        "resolution",
				Type:        "string",
				How:         "--input resolution=<value>",
				Description: "Flux 2 output resolution. The backend default is 1 MP.",
				Example:     "--input resolution=\"1 MP\"",
			},
			{
				Name:        "output_format",
				Type:        "string",
				How:         "--input output_format=webp|png|jpg",
				Description: "Requested output image format.",
				Example:     "--input output_format=png",
			},
			{
				Name:        "output_quality",
				Type:        "integer",
				How:         "--input output_quality=<1-100>",
				Description: "Requested output quality for compressed image formats.",
				Example:     "--input output_quality=90",
			},
			{
				Name:        "safety_tolerance",
				Type:        "integer",
				How:         "--input safety_tolerance=<1-6>",
				Description: "Replicate Flux safety tolerance passed through to the provider.",
				Example:     "--input safety_tolerance=2",
			},
			{
				Name:        "prompt_upsampling",
				Type:        "boolean",
				How:         "--input prompt_upsampling=true|false",
				Description: "Whether the provider should expand the prompt before generation.",
				Example:     "--input prompt_upsampling=false",
			},
		}
	case "suno":
		return []settingHelp{
			{
				Name:        "lyrics",
				Type:        "string",
				How:         "--input lyrics=<text>",
				Description: "Optional lyrics. When present, the prompt is used as style/title context.",
				Example:     "--input lyrics=\"Verse one...\"",
			},
			{
				Name:        "style",
				Type:        "string",
				How:         "--input style=<description>",
				Description: "Musical style prompt for custom-mode songs.",
				Example:     "--input style=\"cinematic electronic\"",
			},
			{
				Name:        "model",
				Type:        "string",
				How:         "--input model=<provider-model>",
				Description: "Optional Suno provider model override; omit it to use the backend default.",
				Example:     "--input model=V5_5",
			},
		}
	}

	if strings.EqualFold(row.Provider, "replicate") {
		return []settingHelp{
			{
				Name:        "additional_provider_inputs",
				Type:        "object",
				How:         "--input key=value or --settings '{...}'",
				Description: "Provider-specific inputs can be passed through when the backend supports them for this action.",
				Example:     "--input seed=42",
			},
		}
	}
	return nil
}

func generationExamplesFor(row generationActionRow) []string {
	ext := defaultOutputExtension(row.Kind)
	examples := []string{fmt.Sprintf("treectl generate %s \"prompt\" --out output.%s", row.Action, ext)}

	switch normalizedActionName(row.Action) {
	case "flux2":
		examples = append(examples, fmt.Sprintf("treectl generate %s \"wide hero banner\" --out banner.webp --input aspect_ratio=3:1", row.Action))
	case "suno":
		examples = append(examples,
			fmt.Sprintf("treectl generate %s \"warm ambient build, 122 BPM\" --duration 22 --out sketch.mp3", row.Action),
			fmt.Sprintf("treectl generate %s \"cinematic electronic\" --instrumental --reference run:abc123 --out track.mp3", row.Action),
		)
	default:
		if row.AcceptsReference {
			examples = append(examples, fmt.Sprintf("treectl generate %s \"prompt\" --reference @reference.png --out output.%s", row.Action, ext))
		}
	}

	examples = append(examples, fmt.Sprintf("treectl generate %s \"prompt\" --quote", row.Action))
	return examples
}

func directGenerationNotesFor(row generationActionRow) []string {
	notes := []string{
		"Use `--quote` to estimate price before generating.",
		"Use repeated `--input key=value` flags for settings; values parse as JSON when possible.",
		"Use `--settings '{...}'` when an agent already has a JSON settings object.",
	}
	if row.Async {
		notes = append(notes, "This action can run asynchronously; treectl polls until completion using --poll-interval and --timeout.")
	}
	return notes
}

func defaultOutputExtension(kind string) string {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "audio":
		return "mp3"
	case "video":
		return "mp4"
	default:
		return "png"
	}
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
