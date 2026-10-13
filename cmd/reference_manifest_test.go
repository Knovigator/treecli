package cmd

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	treeclicontent "github.com/Knovigator/treecli/content"
	"github.com/spf13/cobra"
)

func TestReferenceManifestUsesCanonicalContentAndCobraCompletion(t *testing.T) {
	root := &cobra.Command{Use: "treecli"}
	root.AddCommand(&cobra.Command{Use: "test-visible-command", Short: "A registered command", Run: func(*cobra.Command, []string) {}})
	export := &cobra.Command{Use: "export-reference-manifest", RunE: ReferenceManifestCmd.RunE}
	export.Flags().String("source-revision", strings.Repeat("a", 40), "revision")
	root.AddCommand(export)
	var output bytes.Buffer
	export.SetOut(&output)
	if err := export.RunE(export, nil); err != nil {
		t.Fatal(err)
	}
	var manifest struct {
		SourceRevision  string              `json:"source_revision"`
		Skills          []map[string]string `json:"skills"`
		Completion      map[string]string   `json:"completion"`
		PlainCompletion map[string]string   `json:"completion_no_descriptions"`
	}
	if err := json.Unmarshal(output.Bytes(), &manifest); err != nil {
		t.Fatal(err)
	}
	skills, err := treeclicontent.ListPackagedSkills()
	if err != nil {
		t.Fatal(err)
	}
	if manifest.SourceRevision != strings.Repeat("a", 40) || len(manifest.Skills) != len(skills) {
		t.Fatal("missing provenance or skills")
	}
	if manifest.Skills[0]["skill_md"] != skills[0].SkillMD {
		t.Fatal("skill source drift")
	}

	for _, shell := range []string{"bash", "zsh", "fish", "powershell"} {

		for _, plain := range []bool{false, true} {
			var expected bytes.Buffer
			if err := RenderCompletion(root, shell, &expected, plain); err != nil {
				t.Fatal(err)
			}

			got := manifest.Completion[shell]
			if plain {
				got = manifest.PlainCompletion[shell]
			}
			if got == "" || got != expected.String() {
				t.Fatalf("%s plain=%v completion differs from registered command", shell, plain)
			}
		}
	}

	if len(manifest.Completion) != 4 {
		t.Fatal("missing completion shells")
	}
}
