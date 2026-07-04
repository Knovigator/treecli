package cmd

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"mime"
	neturl "net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"

	"github.com/Knovigator/treecli/api"
)

type streamTarget struct {
	Kind string
	ID   string
	Name string
}

func clipLink(url, content, attachment string, target streamTarget, outputFormat string) error {
	profile, err := requireAuthenticatedProfile()
	if err != nil {
		return err
	}

	result, err := createClipQuest(profile, url, content, attachment, target)
	if err != nil {
		return fmt.Errorf("creating post: %w", err)
	}

	return printCreateQuestResult(profile, result, outputFormat)
}

func resolveSpaceID(profile profileConfig, explicitSpaceID string) (string, error) {
	spaceID := strings.TrimSpace(explicitSpaceID)
	if spaceID != "" {
		return spaceID, nil
	}

	if strings.TrimSpace(profile.ActiveSpaceID) != "" {
		return strings.TrimSpace(profile.ActiveSpaceID), nil
	}

	return "", fmt.Errorf("missing space_id; pass --space-id or re-run treecli login for this profile")
}

func textToDeltaJSONString(content string) (string, error) {
	if strings.TrimSpace(content) == "" {
		return "", nil
	}

	return marshalDeltaOps(buildPlainTextDeltaOps(content))
}

func actionTextToDeltaJSONString(content string) (string, error) {
	if strings.TrimSpace(content) == "" {
		return "", nil
	}

	return marshalDeltaOps(buildActionTextDeltaOps(content))
}

func marshalDeltaOps(ops []map[string]interface{}) (string, error) {
	deltaPayload := map[string][]map[string]interface{}{
		"ops": ops,
	}

	deltaJSON, err := json.Marshal(deltaPayload)
	if err != nil {
		return "", err
	}

	return string(deltaJSON), nil
}

func buildPlainTextDeltaOps(content string) []map[string]interface{} {
	return []map[string]interface{}{
		{"insert": content},
	}
}

func buildActionTextDeltaOps(content string) []map[string]interface{} {
	actionTagPattern := regexp.MustCompile(`\B!\w+`)
	matches := actionTagPattern.FindAllStringIndex(content, -1)
	if len(matches) == 0 {
		return buildPlainTextDeltaOps(content)
	}

	ops := []map[string]interface{}{}
	lastIndex := 0

	for _, matchRange := range matches {
		startIndex := matchRange[0]
		endIndex := matchRange[1]

		if startIndex > lastIndex {
			ops = append(ops, map[string]interface{}{"insert": content[lastIndex:startIndex]})
		}

		ops = append(
			ops,
			map[string]interface{}{
				"insert": content[startIndex:endIndex],
				"attributes": map[string]bool{
					"bold": true,
				},
			},
		)
		lastIndex = endIndex
	}

	if lastIndex < len(content) {
		ops = append(ops, map[string]interface{}{"insert": content[lastIndex:]})
	}

	return ops
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

func prepareClipAttachmentData(attachmentPath string) ([]byte, []byte, []byte, error) {
	if strings.TrimSpace(attachmentPath) == "" {
		return nil, nil, nil, nil
	}

	fileContent, err := os.ReadFile(attachmentPath)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("error reading attachment file: %w", err)
	}

	mimeType := getMimeType(attachmentPath)
	switch {
	case strings.HasPrefix(mimeType, "image/"):
		return fileContent, nil, nil, nil
	case strings.HasPrefix(mimeType, "video/"):
		return nil, fileContent, nil, nil
	default:
		return nil, nil, fileContent, nil
	}
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

func defaultPrivateStreamTarget() streamTarget {
	return streamTarget{
		Kind: "private",
		ID:   "PSEUDOSTREAM__PRIVATE",
		Name: "Private",
	}
}

func resolveStreamTarget(profile profileConfig, streamValue string, defaultTarget streamTarget) (streamTarget, error) {
	if strings.TrimSpace(streamValue) == "" {
		return defaultTarget, nil
	}

	normalizedStreamValue := normalizeStreamKey(streamValue)
	if pseudoTarget, ok := resolvePseudoStreamTarget(normalizedStreamValue); ok {
		return pseudoTarget, nil
	}

	spaceID, err := resolveSpaceID(profile, "")
	if err != nil {
		return streamTarget{}, err
	}

	userTeams, err := api.ListTeams(profile.BackendURL, profile.AccessToken, profile.Client, profile.UID, spaceID, false)
	if err != nil {
		return streamTarget{}, err
	}

	orderedTeams := appendUniqueTeams(nil, userTeams.Teams)

	publicTeams, err := api.ListPublicTeams(profile.BackendURL, profile.AccessToken, profile.Client, profile.UID, spaceID)
	if err == nil {
		orderedTeams = appendUniqueTeams(orderedTeams, publicTeams.Teams)
	}

	for _, team := range orderedTeams {
		if strings.EqualFold(team.ID, strings.TrimSpace(streamValue)) {
			return streamTarget{Kind: "team", ID: team.ID, Name: team.Name}, nil
		}

		if normalizeStreamKey(team.Name) == normalizedStreamValue {
			return streamTarget{Kind: "team", ID: team.ID, Name: team.Name}, nil
		}

		if normalizeStreamKey(team.ShortName) == normalizedStreamValue {
			return streamTarget{Kind: "team", ID: team.ID, Name: team.Name}, nil
		}
	}

	if looksLikeUUID(streamValue) {
		trimmedValue := strings.TrimSpace(streamValue)
		return streamTarget{Kind: "team", ID: trimmedValue, Name: trimmedValue}, nil
	}

	return streamTarget{}, fmt.Errorf("could not resolve stream %q", streamValue)
}

func appendUniqueTeams(existing []api.TeamRef, additional []api.TeamRef) []api.TeamRef {
	seenTeamIDs := map[string]bool{}
	for _, team := range existing {
		if strings.TrimSpace(team.ID) == "" {
			continue
		}
		seenTeamIDs[team.ID] = true
	}

	for _, team := range additional {
		if strings.TrimSpace(team.ID) == "" || seenTeamIDs[team.ID] {
			continue
		}
		existing = append(existing, team)
		seenTeamIDs[team.ID] = true
	}

	return existing
}

func resolvePseudoStreamTarget(normalizedStreamValue string) (streamTarget, bool) {
	switch normalizedStreamValue {
	case "private", "pseudostreamprivate":
		return streamTarget{Kind: "private", ID: "PSEUDOSTREAM__PRIVATE", Name: "Private"}, true
	case "public", "home", "homepublic", "pseudostreampublic":
		return streamTarget{Kind: "public", ID: "PSEUDOSTREAM__PUBLIC", Name: "Home (Public)"}, true
	case "clips", "clip", "pseudostreamclips":
		return streamTarget{Kind: "clips", ID: "PSEUDOSTREAM__CLIPS", Name: "Clips"}, true
	default:
		return streamTarget{}, false
	}
}

func normalizeStreamKey(value string) string {
	trimmedValue := strings.TrimSpace(strings.ToLower(value))
	return strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return r
		}
		return -1
	}, trimmedValue)
}

func looksLikeUUID(value string) bool {
	uuidPattern := regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
	return uuidPattern.MatchString(strings.TrimSpace(value))
}

func normalizeReplyTarget(value string) (string, error) {
	trimmedValue := strings.TrimSpace(value)
	if trimmedValue == "" {
		return "", fmt.Errorf("missing --reply-to value")
	}

	questID, ok := extractQuestIDFromTarget(trimmedValue)
	if !ok {
		return "", fmt.Errorf("--reply-to must be a quest UUID or quest link")
	}

	return questID, nil
}

func normalizeThreadTarget(value string) (string, error) {
	trimmedValue := strings.TrimSpace(value)
	if trimmedValue == "" {
		return "", fmt.Errorf("missing quest target")
	}

	questID, ok := extractQuestIDFromTarget(trimmedValue)
	if !ok {
		return "", fmt.Errorf("thread target must be a quest UUID or quest link")
	}

	return questID, nil
}

func normalizeAnswerTarget(value string) (string, error) {
	trimmedValue := strings.TrimSpace(value)
	if trimmedValue == "" {
		return "", fmt.Errorf("missing post target")
	}

	if looksLikeUUID(trimmedValue) {
		return trimmedValue, nil
	}

	parsedURL, err := parseTreechatTargetURL(trimmedValue)
	if err != nil {
		return "", fmt.Errorf("post target must be a post UUID or post link")
	}

	if _, ok := extractQuestIDFromParsedURL(parsedURL); ok {
		return "", fmt.Errorf("post target must be a post UUID or post link; use the default quest target for thread links")
	}

	answerID, ok := extractFirstUUIDFromParsedURL(parsedURL)
	if !ok {
		return "", fmt.Errorf("post target must be a post UUID or post link")
	}

	return answerID, nil
}

func extractQuestIDFromTarget(value string) (string, bool) {
	trimmedValue := strings.TrimSpace(value)
	if looksLikeUUID(trimmedValue) {
		return trimmedValue, true
	}

	parsedURL, err := parseTreechatTargetURL(trimmedValue)
	if err != nil {
		return "", false
	}

	return extractQuestIDFromParsedURL(parsedURL)
}

func parseTreechatTargetURL(value string) (*neturl.URL, error) {
	candidateValue := strings.TrimSpace(value)
	if strings.HasPrefix(candidateValue, "/") {
		candidateValue = "https://placeholder.invalid" + candidateValue
	} else if !strings.Contains(candidateValue, "://") && strings.Contains(candidateValue, "/") {
		candidateValue = "https://" + candidateValue
	}

	return neturl.Parse(candidateValue)
}

func extractQuestIDFromParsedURL(parsedURL *neturl.URL) (string, bool) {
	pathSegments := strings.Split(strings.Trim(parsedURL.Path, "/"), "/")
	for index := 0; index < len(pathSegments)-1; index++ {
		if pathSegments[index] != "quest" {
			continue
		}

		candidateID := strings.TrimSpace(pathSegments[index+1])
		if looksLikeUUID(candidateID) {
			return candidateID, true
		}
	}

	return "", false
}

func extractFirstUUIDFromParsedURL(parsedURL *neturl.URL) (string, bool) {
	searchParts := []string{
		parsedURL.Path,
		parsedURL.Fragment,
		parsedURL.RawQuery,
	}

	for _, searchPart := range searchParts {
		candidateIDs := strings.FieldsFunc(searchPart, func(r rune) bool {
			return !(unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-')
		})
		for _, candidateID := range candidateIDs {
			if looksLikeUUID(candidateID) {
				return candidateID, true
			}
		}
	}

	return "", false
}

func createClipQuest(
	profile profileConfig,
	url string,
	content string,
	attachment string,
	target streamTarget,
) (api.CreateQuestResponse, error) {
	image, video, file, err := prepareClipAttachmentData(attachment)
	if err != nil {
		return api.CreateQuestResponse{}, err
	}

	deltaJSON, err := textToDeltaJSONString(content)
	if err != nil {
		return api.CreateQuestResponse{}, err
	}

	return api.CreateClipQuest(
		profile.BackendURL,
		profile.AccessToken,
		profile.Client,
		profile.UID,
		api.CreateClipQuestRequest{
			URL:         url,
			Content:     content,
			DeltaJSON:   deltaJSON,
			Image:       image,
			Video:       video,
			File:        file,
			Destination: clipDestinationFromTarget(target),
		},
	)
}

func clipDestinationFromTarget(target streamTarget) map[string]interface{} {
	return map[string]interface{}{
		"type": "stream",
		"id":   target.ID,
		"name": target.Name,
	}
}

func printCreateQuestResult(profile profileConfig, result api.CreateQuestResponse, outputFormat string) error {
	if outputFormat == "json" {
		prettyJSON, err := api.PrettyJSON(result.Raw)
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
	fmt.Printf("Link: %s\n", threadLink(profile, result.Quest.ID))
	return nil
}

func printCreateAnswerResult(profile profileConfig, result api.CreateAnswerResponse, outputFormat string) error {
	if outputFormat == "json" {
		prettyJSON, err := api.PrettyJSON(result.Raw)
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
