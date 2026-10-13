package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"

	treeclicontent "github.com/Knovigator/treecli/content"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// ReferenceManifestCmd exports generated MCP resources from the CLI's canonical
// embedded content and the fully registered command tree. No profiles are read.
var ReferenceManifestCmd = &cobra.Command{
	Use:    "export-reference-manifest",
	Hidden: true,
	Args:   cobra.NoArgs,
	RunE: func(command *cobra.Command, args []string) error {
		revision, _ := command.Flags().GetString("source-revision")
		if !regexp.MustCompile(`^[0-9a-f]{40}$`).MatchString(revision) {
			return fmt.Errorf("--source-revision must be a full source commit SHA")
		}
		skills, err := treeclicontent.ListPackagedSkills()
		if err != nil {
			return err
		}
		records := make([]map[string]string, 0, len(skills))
		for _, skill := range skills {
			records = append(records, map[string]string{"key": skill.Key, "name": skill.Name, "description": skill.Description, "skill_md": skill.SkillMD, "openai_yaml": skill.OpenAIYAML})
		}
		long, err := treeclicontent.OnboardLong()
		if err != nil {
			return err
		}
		short, err := treeclicontent.OnboardShort()
		if err != nil {
			return err
		}
		guideLong, err := treeclicontent.BuildOnboardContent("long")
		if err != nil {
			return err
		}
		guideShort, err := treeclicontent.BuildOnboardContent("short")
		if err != nil {
			return err
		}
		root := command.Root()
		root.InitDefaultCompletionCmd()
		completions := map[string]string{}
		plainCompletions := map[string]string{}
		for _, shell := range []string{"bash", "zsh", "fish", "powershell"} {
			for _, plain := range []bool{false, true} {
				var output bytes.Buffer
				err = RenderCompletion(root, shell, &output, plain)
				if err != nil {
					return err
				}
				if plain {
					plainCompletions[shell] = output.String()
				} else {
					completions[shell] = output.String()
				}
			}
		}

		manifest := map[string]interface{}{"schema_version": 1, "source_revision": revision, "skills": records, "onboard": map[string]interface{}{"agents": map[string]string{"short": short, "long": long}, "guide": map[string]string{"short": guideShort, "long": guideLong}}, "completion": completions, "completion_no_descriptions": plainCompletions, "commands": referenceCommandInventory(root)}
		encoder := json.NewEncoder(command.OutOrStdout())
		encoder.SetIndent("", "  ")
		return encoder.Encode(manifest)
	},
}

func init() {
	ReferenceManifestCmd.Flags().String("source-revision", "", "Exact source commit used to generate these resources")
}

// Include hidden compatibility commands and inherited flags in the source audit.
func referenceCommandInventory(root *cobra.Command) []map[string]interface{} {
	var records []map[string]interface{}
	var visit func(*cobra.Command)
	visit = func(command *cobra.Command) {
		flags := []map[string]interface{}{}
		command.InitDefaultHelpFlag()
		all := pflag.NewFlagSet(command.Name(), pflag.ContinueOnError)
		all.AddFlagSet(command.InheritedFlags())
		all.AddFlagSet(command.LocalFlags())
		all.VisitAll(func(flag *pflag.Flag) {
			flags = append(flags, map[string]interface{}{"name": flag.Name, "shorthand": flag.Shorthand, "type": flag.Value.Type(), "default": flag.DefValue, "hidden": flag.Hidden, "deprecated": flag.Deprecated})
		})
		records = append(records, map[string]interface{}{"path": command.CommandPath(), "use": command.Use, "aliases": command.Aliases, "hidden": command.Hidden, "flags": flags})
		for _, child := range command.Commands() {
			visit(child)
		}
	}
	visit(root)
	return records
}
