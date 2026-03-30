package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
)

type ThreadResponse struct {
	Quest Quest           `json:"quest"`
	Raw   json.RawMessage `json:"-"`
}

type CreateQuestResponse struct {
	Quest Quest           `json:"quest"`
	Raw   json.RawMessage `json:"-"`
}

type CreateAnswerResponse struct {
	Answer    Answer          `json:"answer"`
	Quest     *Quest          `json:"quest"`
	SideQuest *Quest          `json:"side_quest"`
	Raw       json.RawMessage `json:"-"`
}

type AnswerResponse struct {
	Answer Answer          `json:"answer"`
	Raw    json.RawMessage `json:"-"`
}

type MessagesResponse struct {
	Answers []Answer        `json:"answers"`
	Raw     json.RawMessage `json:"-"`
}

type Quest struct {
	ID                string   `json:"id"`
	SpaceID           string   `json:"space_id"`
	UserID            string   `json:"user_id"`
	Content           string   `json:"content"`
	Path              string   `json:"path"`
	QuestURL          string   `json:"quest_url"`
	ParentID          string   `json:"parent_id"`
	SortedAnswerIDs   []string `json:"sorted_answer_ids"`
	MatchingAnswerIDs []string `json:"matching_answer_ids"`
	Parent            *Answer  `json:"parent"`
	SortedAnswers     []Answer `json:"sorted_answers"`
}

type Answer struct {
	ID                           string              `json:"id"`
	SpaceID                      string              `json:"space_id"`
	QuestID                      string              `json:"quest_id"`
	UserID                       string              `json:"user_id"`
	Content                      string              `json:"content"`
	DisplayContent               string              `json:"display_content"`
	Path                         string              `json:"path"`
	CreatedAt                    string              `json:"created_at"`
	UpdatedAt                    string              `json:"updated_at"`
	MessageType                  string              `json:"message_type"`
	IsPost                       bool                `json:"is_post"`
	IsDraft                      bool                `json:"is_draft"`
	IsSystem                     bool                `json:"is_system"`
	IsClip                       bool                `json:"is_clip"`
	Deleted                      bool                `json:"deleted"`
	RecordingURL                 *string             `json:"recording_url"`
	MP4RecordingURL              *string             `json:"mp4_recording_url"`
	RecordingThumbnailURL        *string             `json:"recording_thumbnail_url"`
	RecordingType                *string             `json:"recording_type"`
	OriginalRecordingType        *string             `json:"original_recording_type"`
	RecordingInfo                json.RawMessage     `json:"recording_info"`
	AnswerImageURL               *string             `json:"answer_image_url"`
	ImagesV2                     []AnswerImageV2     `json:"images_v2"`
	ImageURLs                    []AnswerImageURL    `json:"image_urls"`
	FileURLs                     []AnswerFileURL     `json:"file_urls"`
	IsGeneratingImage            bool                `json:"is_generating_image"`
	ImageGenerationFailed        bool                `json:"image_generation_failed"`
	ImageGenerationFailureReason *string             `json:"image_generation_failure_reason"`
	User                         *User               `json:"user"`
	URL                          *URL                `json:"url"`
	ChildQuests                  []ChildQuest        `json:"child_quests"`
	BsvAttachments               []BsvAttachment     `json:"bsv_attachments"`
	ImageBsvOrdinal              *BsvOrdinal         `json:"image_bsv_ordinal"`
	VideoBsvOrdinal              *BsvOrdinal         `json:"video_bsv_ordinal"`
	PollOpen                     bool                `json:"poll_open"`
	PollTotalVotes               int                 `json:"poll_total_votes"`
	PollUserVoteOptionID         *string             `json:"poll_user_vote_option_id"`
	PollResultsVisible           bool                `json:"poll_results_visible"`
	PollOptionsPayload           []PollOptionPayload `json:"poll_options_payload"`
}

type User struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Path string `json:"path"`
}

type URL struct {
	Address string `json:"address"`
	Title   string `json:"title"`
}

type AnswerFileURL struct {
	URL          string            `json:"url"`
	Name         string            `json:"name"`
	ID           interface{}       `json:"id"`
	ContentType  string            `json:"content_type"`
	PreviewURLs  map[string]string `json:"preview_urls"`
	PreviewURL   *string           `json:"preview_url"`
	AttachmentID string            `json:"attachment_id"`
}

type AnswerImageURL struct {
	ImageURL         string            `json:"image_url"`
	AttachmentID     string            `json:"attachment_id"`
	ImageByteSize    *int64            `json:"image_byte_size"`
	ImageWidth       *int              `json:"image_width"`
	ImageHeight      *int              `json:"image_height"`
	ImageContentType string            `json:"image_content_type"`
	ImageFilename    string            `json:"image_filename"`
	PreviewURLs      map[string]string `json:"preview_urls"`
	PreviewURL       *string           `json:"preview_url"`
}

type AnswerImageV2 struct {
	AttachmentID     string            `json:"attachment_id"`
	BlobID           string            `json:"blob_id"`
	AttachedAt       string            `json:"attached_at"`
	BlobCreatedAt    string            `json:"blob_created_at"`
	Access           string            `json:"access"`
	ImageByteSize    *int64            `json:"image_byte_size"`
	ImageWidth       *int              `json:"image_width"`
	ImageHeight      *int              `json:"image_height"`
	ImageContentType string            `json:"image_content_type"`
	ImageFilename    string            `json:"image_filename"`
	URLs             map[string]string `json:"urls"`
	PreviewURL       *string           `json:"preview_url"`
}

type BsvAttachment struct {
	ID            string `json:"id"`
	Size          *int64 `json:"size"`
	MimeType      string `json:"mime_type"`
	AttachmentID  string `json:"attachment_id"`
	AttachmentURL string `json:"attachment_url"`
}

type BsvOrdinal struct {
	ID                string `json:"id"`
	Origin            string `json:"origin"`
	InscriptionID     string `json:"inscription_id"`
	InscriptionNumber *int64 `json:"inscription_number"`
	ContentType       string `json:"content_type"`
	QuestID           string `json:"quest_id"`
	AnswerID          string `json:"answer_id"`
}

type ChildQuest struct {
	ID       string `json:"id"`
	ParentID string `json:"parent_id"`
	Path     string `json:"path"`
}

type PollOptionPayload struct {
	ID         string   `json:"id"`
	Label      string   `json:"label"`
	Position   int      `json:"position"`
	VotesCount *int     `json:"votes_count"`
	Percentage *float64 `json:"percentage"`
}

func PrettyJSON(raw []byte) (string, error) {
	var out bytes.Buffer
	err := json.Indent(&out, raw, "", "  ")
	if err != nil {
		return "", err
	}

	return out.String(), nil
}

func (answer Answer) ToASCII() string {
	var output strings.Builder

	userName := "Unknown User"
	if answer.User != nil && answer.User.Name != "" {
		userName = answer.User.Name
	}

	content := strings.TrimSpace(answer.DisplayContent)
	if content == "" {
		content = strings.TrimSpace(answer.Content)
	}
	if content == "" {
		content = "(empty)"
	}

	output.WriteString(fmt.Sprintf("%s: %s", userName, content))

	if answer.URL != nil && answer.URL.Address != "" {
		output.WriteString(fmt.Sprintf(" <%s>", answer.URL.Address))
	}

	if answer.IsGeneratingImage {
		output.WriteString(" [generating image]")
	}

	if answer.ImageGenerationFailed {
		reason := "image generation failed"
		if answer.ImageGenerationFailureReason != nil && *answer.ImageGenerationFailureReason != "" {
			reason = *answer.ImageGenerationFailureReason
		}
		output.WriteString(fmt.Sprintf(" [image failed: %s]", reason))
	}

	mediaURLs := answer.MediaURLs()
	if len(mediaURLs) > 0 {
		output.WriteString("\n")
		for _, mediaURL := range mediaURLs {
			output.WriteString(fmt.Sprintf("  media: %s\n", mediaURL))
		}
	} else {
		output.WriteString("\n")
	}

	return strings.TrimRight(output.String(), "\n")
}

func (answer Answer) MediaURLs() []string {
	urls := []string{}
	seen := map[string]bool{}

	appendURL := func(value string) {
		trimmedValue := strings.TrimSpace(value)
		if trimmedValue == "" || seen[trimmedValue] {
			return
		}
		seen[trimmedValue] = true
		urls = append(urls, trimmedValue)
	}

	if answer.AnswerImageURL != nil {
		appendURL(*answer.AnswerImageURL)
	}

	for _, image := range answer.ImagesV2 {
		for _, imageURL := range image.URLs {
			appendURL(imageURL)
		}
		if image.PreviewURL != nil {
			appendURL(*image.PreviewURL)
		}
	}

	for _, imageURL := range answer.ImageURLs {
		appendURL(imageURL.ImageURL)
		if imageURL.PreviewURL != nil {
			appendURL(*imageURL.PreviewURL)
		}
		for _, previewURL := range imageURL.PreviewURLs {
			appendURL(previewURL)
		}
	}

	for _, fileURL := range answer.FileURLs {
		appendURL(fileURL.URL)
		if fileURL.PreviewURL != nil {
			appendURL(*fileURL.PreviewURL)
		}
		for _, previewURL := range fileURL.PreviewURLs {
			appendURL(previewURL)
		}
	}

	for _, attachment := range answer.BsvAttachments {
		appendURL(attachment.AttachmentURL)
	}

	if answer.RecordingURL != nil {
		appendURL(*answer.RecordingURL)
	}
	if answer.MP4RecordingURL != nil {
		appendURL(*answer.MP4RecordingURL)
	}
	if answer.RecordingThumbnailURL != nil {
		appendURL(*answer.RecordingThumbnailURL)
	}

	return urls
}

func (answer Answer) CanonicalMediaURLs(fallbackBase string) []string {
	mediaURLs := answer.MediaURLs()
	canonicalURLs := make([]string, 0, len(mediaURLs))

	for _, mediaURL := range mediaURLs {
		canonicalURLs = append(canonicalURLs, canonicalizeURL(mediaURL, fallbackBase))
	}

	return canonicalURLs
}

func (answer Answer) GenerationStatus() string {
	if answer.ImageGenerationFailed {
		return "failed"
	}

	if len(answer.MediaURLs()) > 0 {
		return "completed"
	}

	return "pending"
}

func (answer Answer) GenerationFailureReason() string {
	if answer.ImageGenerationFailureReason == nil {
		return ""
	}

	return strings.TrimSpace(*answer.ImageGenerationFailureReason)
}

func canonicalizeURL(rawURL string, fallbackBase string) string {
	trimmedURL := strings.TrimSpace(rawURL)
	if trimmedURL == "" {
		return ""
	}

	if strings.HasPrefix(trimmedURL, "http://") || strings.HasPrefix(trimmedURL, "https://") {
		return trimmedURL
	}

	scheme := "https"
	if strings.HasPrefix(strings.TrimSpace(fallbackBase), "http://") {
		scheme = "http"
	}

	if strings.HasPrefix(trimmedURL, "//") {
		return scheme + ":" + trimmedURL
	}

	if strings.HasPrefix(trimmedURL, "/") {
		return strings.TrimRight(strings.TrimSpace(fallbackBase), "/") + trimmedURL
	}

	return trimmedURL
}

func (quest Quest) ToASCII() string {
	var output strings.Builder

	if quest.Parent != nil {
		output.WriteString(quest.Parent.ToASCII())
		output.WriteString("\n")
	}

	for index, answer := range quest.SortedAnswers {
		if index > 0 {
			output.WriteString("\n")
		}
		output.WriteString(answer.ToASCII())
	}

	return strings.TrimSpace(output.String())
}
