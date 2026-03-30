package content

import (
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

//go:embed onboard/*.md skills/*/SKILL.md skills/*/agents/openai.yaml
var packagedFiles embed.FS

type PackagedSkill struct {
	Key         string
	Name        string
	Description string
	SkillMD     string
	OpenAIYAML  string
}

func OnboardLong() (string, error) {
	return readPackagedText("onboard/agents-long.md")
}

func OnboardShort() (string, error) {
	return readPackagedText("onboard/agents-short.md")
}

func OnboardWrapper() (string, error) {
	return readPackagedText("onboard/wrapper.md")
}

func BuildOnboardContent(mode string) (string, error) {
	wrapper, err := OnboardWrapper()
	if err != nil {
		return "", err
	}

	agentsContent, err := OnboardLong()
	if err != nil {
		return "", err
	}

	if strings.EqualFold(strings.TrimSpace(mode), "short") {
		agentsContent, err = OnboardShort()
		if err != nil {
			return "", err
		}
	}

	return strings.Replace(wrapper, "{{AGENTS_MD_BLOCK}}", agentsContent, 1), nil
}

func ListPackagedSkills() ([]PackagedSkill, error) {
	entries, err := fs.ReadDir(packagedFiles, "skills")
	if err != nil {
		return nil, fmt.Errorf("listing packaged skills: %w", err)
	}

	skills := []PackagedSkill{}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		skill, err := loadPackagedSkill(entry.Name())
		if err != nil {
			return nil, err
		}
		skills = append(skills, skill)
	}

	sort.Slice(skills, func(left int, right int) bool {
		return skills[left].Key < skills[right].Key
	})

	return skills, nil
}

func GetPackagedSkill(key string) (*PackagedSkill, error) {
	skills, err := ListPackagedSkills()
	if err != nil {
		return nil, err
	}

	normalizedKey := strings.TrimSpace(key)
	for _, skill := range skills {
		if skill.Key == normalizedKey {
			skillCopy := skill
			return &skillCopy, nil
		}
	}

	return nil, nil
}

func InstallPackagedSkill(skill PackagedSkill, skillsRootDir string) (string, error) {
	skillDir := filepath.Join(skillsRootDir, skill.Name)
	agentsDir := filepath.Join(skillDir, "agents")

	if err := os.MkdirAll(agentsDir, 0755); err != nil {
		return "", fmt.Errorf("creating skill install directory %s: %w", agentsDir, err)
	}

	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(skill.SkillMD), 0644); err != nil {
		return "", fmt.Errorf("writing packaged skill markdown for %s: %w", skill.Name, err)
	}

	if err := os.WriteFile(filepath.Join(agentsDir, "openai.yaml"), []byte(skill.OpenAIYAML), 0644); err != nil {
		return "", fmt.Errorf("writing packaged agent yaml for %s: %w", skill.Name, err)
	}

	return skillDir, nil
}

func loadPackagedSkill(key string) (PackagedSkill, error) {
	skillMD, err := readPackagedText(fmt.Sprintf("skills/%s/SKILL.md", key))
	if err != nil {
		return PackagedSkill{}, err
	}

	openAIYAML, err := readPackagedText(fmt.Sprintf("skills/%s/agents/openai.yaml", key))
	if err != nil {
		return PackagedSkill{}, err
	}

	name, err := parseFrontmatterValue(skillMD, "name")
	if err != nil {
		return PackagedSkill{}, err
	}

	description, err := parseFrontmatterValue(skillMD, "description")
	if err != nil {
		return PackagedSkill{}, err
	}

	return PackagedSkill{
		Key:         key,
		Name:        name,
		Description: description,
		SkillMD:     skillMD,
		OpenAIYAML:  openAIYAML,
	}, nil
}

func readPackagedText(path string) (string, error) {
	bytes, err := packagedFiles.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("reading packaged content %s: %w", path, err)
	}

	return string(bytes), nil
}

func parseFrontmatterValue(markdown string, key string) (string, error) {
	if !strings.HasPrefix(markdown, "---\n") {
		return "", fmt.Errorf("missing frontmatter in packaged skill for key %q", key)
	}

	parts := strings.SplitN(markdown, "\n---\n", 2)
	if len(parts) != 2 {
		return "", fmt.Errorf("invalid frontmatter in packaged skill for key %q", key)
	}

	frontmatter := strings.TrimPrefix(parts[0], "---\n")
	for _, line := range strings.Split(frontmatter, "\n") {
		if strings.HasPrefix(line, key+":") {
			return strings.TrimSpace(strings.TrimPrefix(line, key+":")), nil
		}
	}

	return "", fmt.Errorf("missing frontmatter key %q in packaged skill", key)
}
