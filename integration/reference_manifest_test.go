package integration_test

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestCompiledReferenceManifestMatchesCompletionCommands(t *testing.T) {
	binary := buildCLI(t, repositoryRoot(t))
	config := t.TempDir()
	output, stderr, err := runCLI(t, binary, config, "", "export-reference-manifest", "--source-revision", strings.Repeat("a", 40))
	if err != nil {
		t.Fatalf("export failed: %v %s", err, stderr)
	}
	var manifest struct {
		Completion map[string]string `json:"completion"`
		Plain      map[string]string `json:"completion_no_descriptions"`
	}
	if err := json.Unmarshal([]byte(output), &manifest); err != nil {
		t.Fatalf("manifest must be standalone JSON: %v", err)
	}
	for _, shell := range []string{"bash", "zsh", "fish", "powershell"} {
		for _, plain := range []bool{false, true} {
			args := []string{"completion", shell}
			want := manifest.Completion[shell]
			if plain {
				args = append(args, "--no-descriptions")
				want = manifest.Plain[shell]
			}
			got, stderr, err := runCLI(t, binary, config, "", args...)
			if err != nil || want == "" || got != want {
				t.Fatalf("%s plain=%v completion mismatch: err=%v stderr=%s", shell, plain, err, stderr)
			}
		}
	}
}
