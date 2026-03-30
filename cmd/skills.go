package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	treectlcontent "github.com/Knovigator/knovigator/treectl/content"
	"github.com/spf13/cobra"
)

var SkillsCmd = &cobra.Command{
	Use:   "skills",
	Short: "List, emit, and install packaged treectl skills",
}

var skillsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List packaged skills embedded in treectl",
	Run:   runSkillsList,
}

var skillsEmitCmd = &cobra.Command{
	Use:   "emit <name>",
	Short: "Print packaged skill content",
	Args:  cobra.ExactArgs(1),
	Run:   runSkillsEmit,
}

var skillsInstallCmd = &cobra.Command{
	Use:   "install [name]",
	Short: "Install packaged skill content to a target skills directory",
	Args:  cobra.MaximumNArgs(1),
	Run:   runSkillsInstall,
}

var skillsEmitFormat string
var skillsInstallDir string
var skillsInstallClaude bool
var skillsInstallCodex bool
var skillsInstallPi bool

func init() {
	skillsEmitCmd.Flags().StringVar(&skillsEmitFormat, "format", "skill-md", "Output format: skill-md or openai-yaml")
	skillsInstallCmd.Flags().StringVar(&skillsInstallDir, "dir", "", "Skills directory root (contains skill folders)")
	skillsInstallCmd.Flags().BoolVar(&skillsInstallClaude, "claude", false, "Install to ~/.claude/skills")
	skillsInstallCmd.Flags().BoolVar(&skillsInstallCodex, "codex", false, "Install to ~/.codex/skills")
	skillsInstallCmd.Flags().BoolVar(&skillsInstallPi, "pi", false, "Install to ~/.pi/agent/skills")

	SkillsCmd.AddCommand(skillsListCmd)
	SkillsCmd.AddCommand(skillsEmitCmd)
	SkillsCmd.AddCommand(skillsInstallCmd)
}

func runSkillsList(cmd *cobra.Command, args []string) {
	skills, err := treectlcontent.ListPackagedSkills()
	if err != nil {
		fmt.Println("Error:", err)
		return
	}

	fmt.Println("Packaged skills:")
	for _, skill := range skills {
		fmt.Printf("- %s: %s\n", skill.Key, skill.Description)
	}
	fmt.Println()
	fmt.Println("Convenience install targets:")
	fmt.Printf("- --claude -> %s\n", filepath.Join(userHomeDir(), ".claude", "skills"))
	fmt.Printf("- --codex  -> %s\n", filepath.Join(userHomeDir(), ".codex", "skills"))
	fmt.Printf("- --pi     -> %s\n", filepath.Join(userHomeDir(), ".pi", "agent", "skills"))
}

func runSkillsEmit(cmd *cobra.Command, args []string) {
	format := strings.TrimSpace(skillsEmitFormat)
	if format != "skill-md" && format != "openai-yaml" {
		fmt.Println("Error: invalid --format. Use 'skill-md' or 'openai-yaml'.")
		return
	}

	skillNames, err := resolvePackagedSkillKeys(args[0])
	if err != nil {
		fmt.Println("Error:", err)
		return
	}

	for index, skillKey := range skillNames {
		skill, err := treectlcontent.GetPackagedSkill(skillKey)
		if err != nil {
			fmt.Println("Error:", err)
			return
		}
		if skill == nil {
			fmt.Printf("Error: unknown skill %q\n", skillKey)
			return
		}

		if len(skillNames) > 1 && index > 0 {
			fmt.Println()
		}
		if len(skillNames) > 1 {
			fmt.Printf("### %s (%s)\n", skill.Key, format)
		}

		if format == "openai-yaml" {
			fmt.Print(skill.OpenAIYAML)
		} else {
			fmt.Print(skill.SkillMD)
		}

		if !strings.HasSuffix(skill.SkillMD, "\n") && format == "skill-md" {
			fmt.Println()
		}
		if !strings.HasSuffix(skill.OpenAIYAML, "\n") && format == "openai-yaml" {
			fmt.Println()
		}
	}
}

func runSkillsInstall(cmd *cobra.Command, args []string) {
	skillKey := "all"
	if len(args) == 1 {
		skillKey = args[0]
	}

	installRoots := resolveSkillInstallRoots()
	if len(installRoots) == 0 {
		fmt.Println("Error: specify at least one target with --dir, --claude, --codex, or --pi.")
		return
	}

	skillKeys, err := resolvePackagedSkillKeys(skillKey)
	if err != nil {
		fmt.Println("Error:", err)
		return
	}

	for _, root := range installRoots {
		fmt.Printf("Installing to %s\n", root)
		for _, key := range skillKeys {
			skill, err := treectlcontent.GetPackagedSkill(key)
			if err != nil {
				fmt.Println("Error:", err)
				return
			}
			if skill == nil {
				fmt.Printf("Error: unknown skill %q\n", key)
				return
			}

			installedDir, err := treectlcontent.InstallPackagedSkill(*skill, root)
			if err != nil {
				fmt.Println("Error:", err)
				return
			}
			fmt.Printf("Installed %s -> %s\n", skill.Key, installedDir)
		}
	}
}

func resolvePackagedSkillKeys(requested string) ([]string, error) {
	skills, err := treectlcontent.ListPackagedSkills()
	if err != nil {
		return nil, err
	}

	if requested == "all" {
		keys := make([]string, 0, len(skills))
		for _, skill := range skills {
			keys = append(keys, skill.Key)
		}
		return keys, nil
	}

	return []string{strings.TrimSpace(requested)}, nil
}

func resolveSkillInstallRoots() []string {
	roots := []string{}
	if strings.TrimSpace(skillsInstallDir) != "" {
		roots = append(roots, strings.TrimSpace(skillsInstallDir))
	}
	if skillsInstallClaude {
		roots = append(roots, filepath.Join(userHomeDir(), ".claude", "skills"))
	}
	if skillsInstallCodex {
		roots = append(roots, filepath.Join(userHomeDir(), ".codex", "skills"))
	}
	if skillsInstallPi {
		roots = append(roots, filepath.Join(userHomeDir(), ".pi", "agent", "skills"))
	}

	seenRoots := map[string]bool{}
	uniqueRoots := []string{}
	for _, root := range roots {
		if root == "" || seenRoots[root] {
			continue
		}
		seenRoots[root] = true
		uniqueRoots = append(uniqueRoots, root)
	}

	return uniqueRoots
}

func userHomeDir() string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return ""
	}

	return homeDir
}
