package cmd

import (
	"crypto/rand"
	"fmt"
	"mime"
	"os"
	"path/filepath"
	"strings"

	"github.com/Knovigator/knovigator/treectl/api"
)

func clipLink(url, content, attachment string, isClip bool) {
	profile, err := requireAuthenticatedProfile()
	if err != nil {
		fmt.Println("Error:", err)
		return
	}

	// set the destination to a stream with id 'PSEUDOSTREAM__CLIPS' for clips, or 'PSEUDOSTREAM__POSTS' for posts
	destinationName := "Clips"
	destinationId := "PSEUDOSTREAM__CLIPS"
	if !isClip {
		destinationName = "Posts"
		destinationId = "PSEUDOSTREAM__POSTS"
	}

	destination := map[string]interface{}{
		"type": "stream",
		"name": destinationName,
		"id":   destinationId,
	}

	var image, video, file []byte

	if attachment != "" {
		fileContent, err := os.ReadFile(attachment)
		if err != nil {
			fmt.Printf("Error reading attachment file: %v\n", err)
			return
		}

		mimeType := getMimeType(attachment)
		switch {
		case strings.HasPrefix(mimeType, "image/"):
			image = fileContent
		case strings.HasPrefix(mimeType, "video/"):
			video = fileContent
		default:
			file = fileContent
		}
	}

	result, err := api.ClipLink(
		profile.BackendURL,
		profile.AccessToken,
		profile.Client,
		profile.UID,
		url,
		image,
		video,
		file,
		"",
		content,
		destination,
	)

	if err != nil {
		fmt.Println("Error creating post:", err)
		return
	} else {
		fmt.Printf("Post created successfully. See it at: %s\n", threadLink(profile, fmt.Sprintf("%v", result["id"])))
	}
}

func resolveSpaceID(profile profileConfig, explicitSpaceID string) (string, error) {
	spaceID := strings.TrimSpace(explicitSpaceID)
	if spaceID != "" {
		return spaceID, nil
	}

	if strings.TrimSpace(profile.ActiveSpaceID) != "" {
		return strings.TrimSpace(profile.ActiveSpaceID), nil
	}

	return "", fmt.Errorf("missing space_id; pass --space-id or re-run treectl login for this profile")
}

func prepareAttachmentUploads(attachmentPath, imageField, recordingField, fileField string) ([]api.MultipartFile, error) {
	if strings.TrimSpace(attachmentPath) == "" {
		return nil, nil
	}

	fileContent, err := os.ReadFile(attachmentPath)
	if err != nil {
		return nil, fmt.Errorf("error reading attachment file: %w", err)
	}

	fieldName := fileField
	mimeType := getMimeType(attachmentPath)
	switch {
	case strings.HasPrefix(mimeType, "image/"):
		fieldName = imageField
	case strings.HasPrefix(mimeType, "video/"):
		fieldName = recordingField
	}

	return []api.MultipartFile{
		{
			FieldName: fieldName,
			FileName:  filepath.Base(attachmentPath),
			Content:   fileContent,
		},
	}, nil
}

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

func printCreateQuestResult(profile profileConfig, result api.CreateQuestResponse, outputFormat string) {
	if outputFormat == "json" {
		prettyJSON, err := api.PrettyJSON(result.Raw)
		if err != nil {
			fmt.Printf("Error formatting JSON: %v\n", err)
			return
		}
		fmt.Println(prettyJSON)
		return
	}

	rootAnswerID := ""
	if result.Quest.Parent != nil {
		rootAnswerID = result.Quest.Parent.ID
	}

	fmt.Printf("Post created. Thread: %s Root answer: %s\n", result.Quest.ID, rootAnswerID)
	fmt.Printf("Link: %s\n", threadLink(profile, result.Quest.ID))
}

func printCreateAnswerResult(profile profileConfig, result api.CreateAnswerResponse, outputFormat string) {
	if outputFormat == "json" {
		prettyJSON, err := api.PrettyJSON(result.Raw)
		if err != nil {
			fmt.Printf("Error formatting JSON: %v\n", err)
			return
		}
		fmt.Println(prettyJSON)
		return
	}

	threadID := result.Answer.QuestID
	if threadID == "" && result.Quest != nil {
		threadID = result.Quest.ID
	}

	fmt.Printf("Reply created. Thread: %s Answer: %s\n", threadID, result.Answer.ID)
	if threadID != "" {
		fmt.Printf("Link: %s\n", threadLink(profile, threadID))
	}
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
