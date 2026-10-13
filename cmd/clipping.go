package cmd

import (
	"crypto/rand"
	"fmt"
	"mime"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/Knovigator/treecli/api"
)

func newUUID() (string, error) {
	randomBytes := make([]byte, 16)
	_, err := rand.Read(randomBytes)
	if err != nil {
		return "", err
	}

	randomBytes[6] = (randomBytes[6] & 0x0f) | 0x40
	randomBytes[8] = (randomBytes[8] & 0x3f) | 0x80

	return fmt.Sprintf(
		"%x-%x-%x-%x-%x",
		randomBytes[0:4],
		randomBytes[4:6],
		randomBytes[6:8],
		randomBytes[8:10],
		randomBytes[10:16],
	), nil
}

func threadLink(profile profileConfig, threadID string) string {
	linkBase := strings.TrimRight(profile.AppHost, "/")
	if linkBase == "" {
		linkBase = strings.TrimRight(profile.BackendURL, "/")
	}

	return fmt.Sprintf("%s/quest/%s", linkBase, threadID)
}

func looksLikeUUID(value string) bool {
	uuidPattern := regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
	return uuidPattern.MatchString(strings.TrimSpace(value))
}

func printCreateQuestResult(profile profileConfig, result api.CreateQuestResponse, outputFormat string) error {
	if outputFormat == "json" {
		prettyJSON, err := prettyWriteSuccessJSON(result.Raw, result.Quest.ID)
		if err != nil {
			return fmt.Errorf("formatting JSON: %w", err)
		}
		fmt.Println(prettyJSON)
		return nil
	}
	if outputFormat != "ascii" {
		return invalidOutputFormatError(outputFormat)
	}

	rootAnswerID := ""
	if result.Quest.Parent != nil {
		rootAnswerID = result.Quest.Parent.ID
	}

	fmt.Printf("Post created. Thread: %s Root answer: %s\n", result.Quest.ID, rootAnswerID)
	fmt.Printf("Write ID: %s\n", result.Quest.ID)
	fmt.Printf("Link: %s\n", threadLink(profile, result.Quest.ID))
	return nil
}

func printCreateAnswerResult(profile profileConfig, result api.CreateAnswerResponse, outputFormat string) error {
	if outputFormat == "json" {
		prettyJSON, err := prettyWriteSuccessJSON(result.Raw, result.Answer.ID)
		if err != nil {
			return fmt.Errorf("formatting JSON: %w", err)
		}
		fmt.Println(prettyJSON)
		return nil
	}
	if outputFormat != "ascii" {
		return invalidOutputFormatError(outputFormat)
	}

	threadID := result.Answer.QuestID
	if threadID == "" && result.Quest != nil {
		threadID = result.Quest.ID
	}

	fmt.Printf("Reply created. Thread: %s Answer: %s\n", threadID, result.Answer.ID)
	fmt.Printf("Write ID: %s\n", result.Answer.ID)
	if threadID != "" {
		fmt.Printf("Link: %s\n", threadLink(profile, threadID))
	}
	return nil
}

func getMimeType(filePath string) string {
	ext := filepath.Ext(filePath)
	mimeType := mime.TypeByExtension(ext)
	if mimeType == "" {
		// if the MIME type is not found, default to application/octet-stream
		mimeType = "application/octet-stream"
	}
	return mimeType
}

func resolveOutputFormat(outputFormat string, jsonRequested bool) string {
	if jsonRequested {
		return "json"
	}

	trimmedOutputFormat := strings.TrimSpace(outputFormat)
	if trimmedOutputFormat == "" {
		return "ascii"
	}

	return trimmedOutputFormat
}

func invalidOutputFormatError(outputFormat string) error {
	return fmt.Errorf("invalid output format: %s. Use 'ascii' or 'json'", outputFormat)
}
