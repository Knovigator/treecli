package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	treeclicontent "github.com/Knovigator/treecli/content"
	"github.com/spf13/cobra"
)

var OnboardCmd = &cobra.Command{
	Use:   "onboard",
	Short: "Check setup status and get next steps for humans and agents",
	Long: `Show treecli setup status and the next steps to finish onboarding.

The bare command prints a checklist: active profile, login state, whether this
directory's agent instruction files (AGENTS.md / CLAUDE.md) carry the treecli
guidance block, and which packaged skills are installed.

Subcommands:
  agents  Print or install the AGENTS.md/CLAUDE.md guidance block
  guide   Print the full onboarding guide document`,
	Example: `  treecli onboard                      # setup checklist with next steps
  treecli onboard --json               # machine-readable status
  treecli onboard agents --write       # install/update the guidance block in AGENTS.md
  treecli onboard agents --check       # verify the guidance block is present and current
  treecli onboard guide                # full onboarding guide document`,
	RunE: runOnboard,
}

var onboardAgentsCmd = &cobra.Command{
	Use:   "agents",
	Short: "Print or install the AGENTS.md/CLAUDE.md guidance block",
	Long: `Print the treecli guidance block for agent instruction files, or install it
idempotently with --write.

--write wraps the block in marker comments and updates it in place on re-runs,
so it never duplicates. Without --file it updates every AGENTS.md/CLAUDE.md in
the current directory that already has the block, appends to an existing
AGENTS.md or CLAUDE.md otherwise, and creates AGENTS.md as a last resort.`,
	Example: `  treecli onboard agents                       # print the block (long variant)
  treecli onboard agents --short               # print the compact variant
  treecli onboard agents --write               # install/update in this directory
  treecli onboard agents --write --file CLAUDE.md
  treecli onboard agents --check               # exit non-zero if missing or stale`,
	Args: cobra.NoArgs,
	RunE: runOnboardAgents,
}

var onboardGuideCmd = &cobra.Command{
	Use:   "guide",
	Short: "Print the full onboarding guide document",
	Long:  "Print the full onboarding guide: install instructions, the Treechat action model, and the agent guidance block.",
	Args:  cobra.NoArgs,
	RunE:  runOnboardGuide,
}

var onboardShort bool
var onboardLong bool
var onboardAgentsMD bool
var onboardOutputPath string
var onboardJSON bool

var onboardAgentsShort bool
var onboardAgentsLong bool
var onboardAgentsWrite bool
var onboardAgentsCheck bool
var onboardAgentsFile string

var onboardGuideShort bool
var onboardGuideLong bool

const onboardBeginMarkerPrefix = "<!-- treecli:onboard:begin"
const onboardEndMarker = "<!-- treecli:onboard:end -->"

var onboardVariantPattern = regexp.MustCompile(`variant=(short|long)`)

var onboardAgentsFileCandidates = []string{"AGENTS.md", "CLAUDE.md"}

type onboardBlockState string

const (
	onboardBlockAbsent  onboardBlockState = "absent"
	onboardBlockCurrent onboardBlockState = "current"
	onboardBlockStale   onboardBlockState = "stale"
)

func init() {
	OnboardCmd.Flags().BoolVar(&onboardJSON, "json", false, "Output status as JSON instead of human-readable text")
	OnboardCmd.Flags().BoolVar(&onboardShort, "short", false, "Use compact onboarding content")
	OnboardCmd.Flags().BoolVar(&onboardLong, "long", false, "Use full onboarding content")
	OnboardCmd.Flags().BoolVar(&onboardAgentsMD, "agents-md", false, "Emit only the agents.md-ready block")
	OnboardCmd.Flags().StringVarP(&onboardOutputPath, "output", "o", "", "Write to file instead of stdout")
	_ = OnboardCmd.Flags().MarkDeprecated("agents-md", "use 'treecli onboard agents'")
	_ = OnboardCmd.Flags().MarkDeprecated("short", "use 'treecli onboard agents --short' or 'treecli onboard guide --short'")
	_ = OnboardCmd.Flags().MarkDeprecated("long", "use 'treecli onboard agents' or 'treecli onboard guide'")
	_ = OnboardCmd.Flags().MarkDeprecated("output", "use 'treecli onboard agents --write' or redirect stdout")

	onboardAgentsCmd.Flags().BoolVar(&onboardAgentsShort, "short", false, "Use the compact guidance block")
	onboardAgentsCmd.Flags().BoolVar(&onboardAgentsLong, "long", false, "Use the full guidance block (default)")
	onboardAgentsCmd.Flags().BoolVar(&onboardAgentsWrite, "write", false, "Install or update the block in an agent instruction file")
	onboardAgentsCmd.Flags().BoolVar(&onboardAgentsCheck, "check", false, "Verify the block is installed and current; exit non-zero otherwise")
	onboardAgentsCmd.Flags().StringVar(&onboardAgentsFile, "file", "", "Target instruction file for --write/--check (default: AGENTS.md/CLAUDE.md in the current directory)")
	onboardAgentsCmd.MarkFlagsMutuallyExclusive("short", "long")
	onboardAgentsCmd.MarkFlagsMutuallyExclusive("write", "check")

	onboardGuideCmd.Flags().BoolVar(&onboardGuideShort, "short", false, "Use the compact guidance block inside the guide")
	onboardGuideCmd.Flags().BoolVar(&onboardGuideLong, "long", false, "Use the full guidance block inside the guide (default)")
	onboardGuideCmd.MarkFlagsMutuallyExclusive("short", "long")

	OnboardCmd.AddCommand(onboardAgentsCmd)
	OnboardCmd.AddCommand(onboardGuideCmd)
}

func runOnboard(cmd *cobra.Command, args []string) error {
	if onboardShort || onboardLong || onboardAgentsMD || onboardOutputPath != "" {
		return runOnboardLegacy()
	}

	status, err := collectOnboardStatus()
	if err != nil {
		return err
	}

	if onboardJSON {
		prettyJSON, err := json.MarshalIndent(status, "", "  ")
		if err != nil {
			return fmt.Errorf("formatting onboarding status: %w", err)
		}
		fmt.Println(string(prettyJSON))
		return nil
	}

	printOnboardStatus(status)
	return nil
}

// runOnboardLegacy preserves the pre-subcommand flag behavior so documented
// invocations like `treecli onboard --agents-md >> AGENTS.md` keep working.
func runOnboardLegacy() error {
	if onboardShort && onboardLong {
		return fmt.Errorf("use only one of --short or --long")
	}

	mode := "long"
	if onboardShort {
		mode = "short"
	}

	var content string
	var err error
	if onboardAgentsMD {
		content, err = onboardAgentsBlock(mode)
	} else {
		content, err = treeclicontent.BuildOnboardContent(mode)
	}
	if err != nil {
		return err
	}

	if onboardOutputPath != "" {
		if err := os.WriteFile(onboardOutputPath, []byte(content), 0644); err != nil {
			return err
		}
		fmt.Printf("Written to %s\n", onboardOutputPath)
		return nil
	}

	printWithTrailingNewline(content)
	return nil
}

func runOnboardGuide(cmd *cobra.Command, args []string) error {
	mode := "long"
	if onboardGuideShort {
		mode = "short"
	}

	content, err := treeclicontent.BuildOnboardContent(mode)
	if err != nil {
		return err
	}

	printWithTrailingNewline(content)
	return nil
}

func runOnboardAgents(cmd *cobra.Command, args []string) error {
	variant := ""
	if onboardAgentsShort {
		variant = "short"
	}
	if onboardAgentsLong {
		variant = "long"
	}

	if onboardAgentsCheck {
		return runOnboardAgentsCheck()
	}
	if onboardAgentsWrite {
		return runOnboardAgentsWrite(variant)
	}

	if variant == "" {
		variant = "long"
	}
	content, err := onboardAgentsBlock(variant)
	if err != nil {
		return err
	}

	printWithTrailingNewline(content)
	return nil
}

func runOnboardAgentsWrite(variant string) error {
	targets, err := resolveOnboardWriteTargets(onboardAgentsFile)
	if err != nil {
		return err
	}

	for _, target := range targets {
		action, err := upsertOnboardBlockInFile(target, variant)
		if err != nil {
			return err
		}
		fmt.Printf("%s: guidance block %s\n", target, action)
	}
	return nil
}

func runOnboardAgentsCheck() error {
	targets := onboardAgentsFileCandidates
	if strings.TrimSpace(onboardAgentsFile) != "" {
		targets = []string{strings.TrimSpace(onboardAgentsFile)}
	}

	anyCurrent := false
	for _, target := range targets {
		state, variant, err := onboardBlockStateInFile(target)
		if err != nil {
			return err
		}

		switch state {
		case onboardBlockCurrent:
			fmt.Printf("%s: guidance block current (%s)\n", target, variant)
			anyCurrent = true
		case onboardBlockStale:
			fmt.Printf("%s: guidance block stale (%s)\n", target, variant)
		case onboardBlockAbsent:
			fmt.Printf("%s: no guidance block\n", target)
		}
	}

	if !anyCurrent {
		return fmt.Errorf("no current treecli guidance block found; run treecli onboard agents --write")
	}
	return nil
}

func onboardAgentsBlock(variant string) (string, error) {
	if strings.EqualFold(strings.TrimSpace(variant), "short") {
		return treeclicontent.OnboardShort()
	}
	return treeclicontent.OnboardLong()
}

func renderManagedOnboardBlock(variant string) (string, error) {
	block, err := onboardAgentsBlock(variant)
	if err != nil {
		return "", err
	}

	beginMarker := fmt.Sprintf("%s variant=%s (managed by `treecli onboard agents --write`; edits inside are overwritten) -->", onboardBeginMarkerPrefix, variant)
	return beginMarker + "\n" + strings.TrimRight(block, "\n") + "\n" + onboardEndMarker, nil
}

// upsertOnboardBlockInFile installs or refreshes the marked guidance block in
// path and reports the action taken: created, appended, updated, or unchanged.
// An empty variant preserves the variant already recorded in the file's
// markers and defaults to long.
func upsertOnboardBlockInFile(path string, variant string) (string, error) {
	existingBytes, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return "", fmt.Errorf("reading %s: %w", path, err)
	}
	existing := string(existingBytes)

	if variant == "" {
		variant = onboardVariantFromMarkers(existing)
	}

	managedBlock, err := renderManagedOnboardBlock(variant)
	if err != nil {
		return "", err
	}

	updated, action, err := upsertOnboardBlock(existing, managedBlock, path)
	if err != nil {
		return "", err
	}
	if action == "unchanged" {
		return action, nil
	}

	if err := os.WriteFile(path, []byte(updated), 0644); err != nil {
		return "", fmt.Errorf("writing %s: %w", path, err)
	}
	return action, nil
}

func upsertOnboardBlock(existing string, managedBlock string, path string) (string, string, error) {
	beginIndex := strings.Index(existing, onboardBeginMarkerPrefix)
	if beginIndex >= 0 {
		endIndex := strings.Index(existing[beginIndex:], onboardEndMarker)
		if endIndex < 0 {
			return "", "", fmt.Errorf("%s has a begin marker without %q; fix the file and re-run", path, onboardEndMarker)
		}
		endIndex = beginIndex + endIndex + len(onboardEndMarker)

		if strings.TrimSpace(existing[beginIndex:endIndex]) == strings.TrimSpace(managedBlock) {
			return existing, "unchanged", nil
		}
		return existing[:beginIndex] + managedBlock + existing[endIndex:], "updated", nil
	}

	if strings.TrimSpace(existing) == "" {
		return managedBlock + "\n", "created", nil
	}

	return strings.TrimRight(existing, "\n") + "\n\n" + managedBlock + "\n", "appended", nil
}

func onboardVariantFromMarkers(content string) string {
	beginIndex := strings.Index(content, onboardBeginMarkerPrefix)
	if beginIndex >= 0 {
		beginLine := content[beginIndex:]
		if newlineIndex := strings.Index(beginLine, "\n"); newlineIndex >= 0 {
			beginLine = beginLine[:newlineIndex]
		}
		if match := onboardVariantPattern.FindStringSubmatch(beginLine); match != nil {
			return match[1]
		}
	}
	return "long"
}

func onboardBlockStateInFile(path string) (onboardBlockState, string, error) {
	contentBytes, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return onboardBlockAbsent, "", nil
	}
	if err != nil {
		return onboardBlockAbsent, "", fmt.Errorf("reading %s: %w", path, err)
	}

	content := string(contentBytes)
	beginIndex := strings.Index(content, onboardBeginMarkerPrefix)
	if beginIndex < 0 {
		return onboardBlockAbsent, "", nil
	}

	endIndex := strings.Index(content[beginIndex:], onboardEndMarker)
	if endIndex < 0 {
		return onboardBlockStale, onboardVariantFromMarkers(content), nil
	}
	endIndex = beginIndex + endIndex + len(onboardEndMarker)

	variant := onboardVariantFromMarkers(content)
	managedBlock, err := renderManagedOnboardBlock(variant)
	if err != nil {
		return onboardBlockAbsent, "", err
	}

	if strings.TrimSpace(content[beginIndex:endIndex]) == strings.TrimSpace(managedBlock) {
		return onboardBlockCurrent, variant, nil
	}
	return onboardBlockStale, variant, nil
}

// resolveOnboardWriteTargets picks the files --write should touch: the
// explicit --file when given; otherwise every candidate that already carries
// the block, then an existing AGENTS.md or CLAUDE.md, then a new AGENTS.md.
func resolveOnboardWriteTargets(explicitFile string) ([]string, error) {
	if strings.TrimSpace(explicitFile) != "" {
		return []string{strings.TrimSpace(explicitFile)}, nil
	}

	withBlock := []string{}
	existing := []string{}
	for _, candidate := range onboardAgentsFileCandidates {
		state, _, err := onboardBlockStateInFile(candidate)
		if err != nil {
			return nil, err
		}
		if state != onboardBlockAbsent {
			withBlock = append(withBlock, candidate)
		}

		if _, err := os.Stat(candidate); err == nil {
			existing = append(existing, candidate)
		}
	}

	if len(withBlock) > 0 {
		return withBlock, nil
	}
	if len(existing) > 0 {
		return existing[:1], nil
	}
	return []string{onboardAgentsFileCandidates[0]}, nil
}

type onboardStatus struct {
	CLIVersion string               `json:"cli_version"`
	Profile    onboardProfileStatus `json:"profile"`
	AgentFiles []onboardFileStatus  `json:"agent_files"`
	Skills     onboardSkillsStatus  `json:"skills"`
	NextSteps  []onboardNextStep    `json:"next_steps"`
}

type onboardProfileStatus struct {
	Name       string `json:"name"`
	BackendURL string `json:"backend_url,omitempty"`
	SignedIn   bool   `json:"signed_in"`
	Error      string `json:"error,omitempty"`
}

type onboardFileStatus struct {
	Path    string `json:"path"`
	Exists  bool   `json:"exists"`
	Block   string `json:"block"`
	Variant string `json:"variant,omitempty"`
}

type onboardSkillsStatus struct {
	Packaged  int                  `json:"packaged"`
	Installed []onboardSkillTarget `json:"installed"`
}

type onboardSkillTarget struct {
	Target string `json:"target"`
	Dir    string `json:"dir"`
	Count  int    `json:"count"`
}

type onboardNextStep struct {
	Description string `json:"description"`
	Command     string `json:"command"`
}

func collectOnboardStatus() (onboardStatus, error) {
	status := onboardStatus{CLIVersion: CurrentVersion}

	profileName := resolveProfileName()
	status.Profile.Name = profileName
	profile, err := resolveProfile(profileName)
	if err != nil {
		status.Profile.Error = err.Error()
	} else {
		status.Profile.BackendURL = profile.BackendURL
		status.Profile.SignedIn = profile.AccessToken != "" && profile.Client != "" && profile.UID != ""
	}

	for _, candidate := range onboardAgentsFileCandidates {
		state, variant, err := onboardBlockStateInFile(candidate)
		if err != nil {
			return onboardStatus{}, err
		}

		_, statErr := os.Stat(candidate)
		status.AgentFiles = append(status.AgentFiles, onboardFileStatus{
			Path:    candidate,
			Exists:  statErr == nil,
			Block:   string(state),
			Variant: variant,
		})
	}

	packagedSkills, err := treeclicontent.ListPackagedSkills()
	if err != nil {
		return onboardStatus{}, err
	}
	status.Skills.Packaged = len(packagedSkills)

	skillTargets := []onboardSkillTarget{
		{Target: "claude", Dir: filepath.Join(userHomeDir(), ".claude", "skills")},
		{Target: "codex", Dir: filepath.Join(userHomeDir(), ".codex", "skills")},
		{Target: "pi", Dir: filepath.Join(userHomeDir(), ".pi", "agent", "skills")},
	}
	for index := range skillTargets {
		for _, skill := range packagedSkills {
			if _, err := os.Stat(filepath.Join(skillTargets[index].Dir, skill.Name, "SKILL.md")); err == nil {
				skillTargets[index].Count++
			}
		}
	}
	status.Skills.Installed = skillTargets

	status.NextSteps = buildOnboardNextSteps(status)
	return status, nil
}

func buildOnboardNextSteps(status onboardStatus) []onboardNextStep {
	steps := []onboardNextStep{}

	if status.Profile.Error != "" {
		steps = append(steps, onboardNextStep{
			Description: fmt.Sprintf("Fix the %q profile (%s)", status.Profile.Name, status.Profile.Error),
			Command:     "treecli profile list",
		})
	} else if !status.Profile.SignedIn {
		steps = append(steps, onboardNextStep{
			Description: fmt.Sprintf("Log in to the %q profile", status.Profile.Name),
			Command:     fmt.Sprintf("treecli login --profile %s", status.Profile.Name),
		})
	}

	hasCurrentBlock := false
	hasStaleBlock := false
	for _, file := range status.AgentFiles {
		if file.Block == string(onboardBlockCurrent) {
			hasCurrentBlock = true
		}
		if file.Block == string(onboardBlockStale) {
			hasStaleBlock = true
		}
	}
	if hasStaleBlock {
		steps = append(steps, onboardNextStep{
			Description: "Refresh the outdated treecli guidance block in this directory",
			Command:     "treecli onboard agents --write",
		})
	} else if !hasCurrentBlock {
		steps = append(steps, onboardNextStep{
			Description: "Add the treecli guidance block to this project's agent instructions",
			Command:     "treecli onboard agents --write",
		})
	}

	skillsInstalledSomewhere := false
	for _, target := range status.Skills.Installed {
		if target.Count > 0 {
			skillsInstalledSomewhere = true
		}
	}
	if !skillsInstalledSomewhere && status.Skills.Packaged > 0 {
		steps = append(steps, onboardNextStep{
			Description: "Install the packaged agent skills (pick --claude, --codex, or --pi)",
			Command:     "treecli skills install all --claude",
		})
	}

	return steps
}

func printOnboardStatus(status onboardStatus) {
	fmt.Println("treecli onboarding")
	fmt.Println()

	fmt.Printf("  [x] treecli %s\n", status.CLIVersion)

	if status.Profile.Error != "" {
		fmt.Printf("  [ ] profile %q: %s\n", status.Profile.Name, status.Profile.Error)
	} else {
		fmt.Printf("  [x] profile: %s (%s)\n", status.Profile.Name, status.Profile.BackendURL)
		fmt.Printf("  %s login: %s\n", checkbox(status.Profile.SignedIn), signedInLabel(status.Profile.SignedIn))
	}

	blockLine := "no guidance block in AGENTS.md or CLAUDE.md here"
	blockDone := false
	for _, file := range status.AgentFiles {
		switch file.Block {
		case string(onboardBlockCurrent):
			blockLine = fmt.Sprintf("agent guidance: %s (current, %s)", file.Path, file.Variant)
			blockDone = true
		case string(onboardBlockStale):
			if !blockDone {
				blockLine = fmt.Sprintf("agent guidance: %s (stale)", file.Path)
			}
		}
	}
	fmt.Printf("  %s %s\n", checkbox(blockDone), blockLine)

	skillsLine := "skills: none installed"
	skillsDone := false
	installedParts := []string{}
	for _, target := range status.Skills.Installed {
		if target.Count > 0 {
			skillsDone = true
			installedParts = append(installedParts, fmt.Sprintf("%d/%d %s", target.Count, status.Skills.Packaged, target.Target))
		}
	}
	if skillsDone {
		skillsLine = "skills: " + strings.Join(installedParts, ", ")
	}
	fmt.Printf("  %s %s\n", checkbox(skillsDone), skillsLine)

	if len(status.NextSteps) > 0 {
		fmt.Println()
		fmt.Println("Next steps:")
		for index, step := range status.NextSteps {
			fmt.Printf("  %d. %s:\n       %s\n", index+1, step.Description, step.Command)
		}
	} else {
		fmt.Println()
		fmt.Println("All set. Try:")
		fmt.Println("  treecli new post \"hello world\"")
		fmt.Println("  treecli action flux \"a glass cathedral in the rain\"")
	}

	fmt.Println()
	fmt.Println("More: treecli onboard guide | treecli onboard agents --help | treecli completion --help")
}

func checkbox(done bool) string {
	if done {
		return "[x]"
	}
	return "[ ]"
}

func signedInLabel(signedIn bool) string {
	if signedIn {
		return "signed in"
	}
	return "signed out"
}

func printWithTrailingNewline(content string) {
	fmt.Print(content)
	if len(content) == 0 || content[len(content)-1] != '\n' {
		fmt.Println()
	}
}
