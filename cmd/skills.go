package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	treeclicontent "github.com/Knovigator/treecli/content"
	"github.com/spf13/cobra"
)

var SkillsCmd = &cobra.Command{
	Use:   "skills",
	Short: "List, emit, and install packaged treecli skills",
}

var skillsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List packaged skills embedded in treecli",
	RunE:  runSkillsList,
}

var skillsEmitCmd = &cobra.Command{
	Use:   "emit <name>",
	Short: "Print packaged skill content",
	Args:  cobra.ExactArgs(1),
	RunE:  runSkillsEmit,
}

var skillsInstallCmd = &cobra.Command{
	Use:   "install [name]",
	Short: "Install packaged skill content to a target skills directory",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runSkillsInstall,
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

func runSkillsList(cmd *cobra.Command, args []string) error {
	skills, err := treeclicontent.ListPackagedSkills()
	if err != nil {
		return err
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
	return nil
}

func runSkillsEmit(cmd *cobra.Command, args []string) error {
	format := strings.TrimSpace(skillsEmitFormat)
	if format != "skill-md" && format != "openai-yaml" {
		return fmt.Errorf("invalid --format. Use 'skill-md' or 'openai-yaml'")
	}

	skillNames, err := resolvePackagedSkillKeys(args[0])
	if err != nil {
		return err
	}

	for index, skillKey := range skillNames {
		skill, err := treeclicontent.GetPackagedSkill(skillKey)
		if err != nil {
			return err
		}
		if skill == nil {
			return fmt.Errorf("unknown skill %q", skillKey)
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
	return nil
}

func runSkillsInstall(cmd *cobra.Command, args []string) error {
	skillKey := "all"
	if len(args) == 1 {
		skillKey = args[0]
	}

	installRoots := resolveSkillInstallRoots()
	if len(installRoots) == 0 {
		return fmt.Errorf("specify at least one target with --dir, --claude, --codex, or --pi")
	}

	skillKeys, err := resolvePackagedSkillKeys(skillKey)
	if err != nil {
		return err
	}

	for _, root := range installRoots {
		fmt.Printf("Installing to %s\n", root)
		for _, key := range skillKeys {
			skill, err := treeclicontent.GetPackagedSkill(key)
			if err != nil {
				return err
			}
			if skill == nil {
				return fmt.Errorf("unknown skill %q", key)
			}

			installedDir, err := treeclicontent.InstallPackagedSkill(*skill, root)
			if err != nil {
				return err
			}
			fmt.Printf("Installed %s -> %s\n", skill.Key, installedDir)
		}
	}
	return nil
}

func resolvePackagedSkillKeys(requested string) ([]string, error) {
	skills, err := treeclicontent.ListPackagedSkills()
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
