package cmd

import (
	"fmt"
	"mime"
	"os"
	"path/filepath"
	"strings"

	"github.com/Knovigator/knovigator/treectl/api"
	"github.com/spf13/viper"
)

func clipLink(url, content, attachment string, isClip bool) {
	// load credentials from viper config
	accessToken := viper.GetString("access_token")
	client := viper.GetString("client")
	uid := viper.GetString("uid")
	backendURL := viper.GetString("backend_url")

	if accessToken == "" || client == "" || uid == "" || backendURL == "" {
		fmt.Println("Error: Missing credentials. Please login first.")
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
	var err error

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
		backendURL,
		accessToken,
		client,
		uid,
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
		fmt.Printf("Post created successfully. See it at: http://home.treechat.ai/quest/%s\n", result["id"])
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
