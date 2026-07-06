package cmd

import (
	"fmt"
	"strings"
)

var legacyReferenceActionBases = map[string]string{
	"animate":           "minimax",
	"animate_grok":      "grokvideo",
	"animate_grok1.5":   "grokvideo1.5",
	"animate_kling":     "kling2",
	"animate_kling3":    "kling3",
	"animate_luma":      "luma",
	"animate_seedance":  "seedance1",
	"animate_seedance2": "seedance2",
	"animate_sora2":     "sora2",
	"animate_veo":       "veo2",
	"animate_veo3":      "veo3",
	"animate_wan":       "wan",
	"edit":              "flash",
	"edit_flux":         "flux",
	"edit_grok":         "grokimage",
	"edit_nb":           "nb",
	"edit_nanobanana":   "nb",
	"edit_nbpro":        "nbpro",
	"edit_openai":       "gpt4o",
	"edit_qwen":         "qwen",
}

func isLegacyReferenceActionName(action string) bool {
	normalized := normalizedActionName(action)
	if normalized == "animate" || normalized == "edit" {
		return true
	}
	return strings.HasPrefix(normalized, "animate_") || strings.HasPrefix(normalized, "edit_")
}

func legacyReferenceActionBase(action string) (string, bool) {
	normalized := normalizedActionName(action)
	if !isLegacyReferenceActionName(normalized) {
		return "", false
	}

	base, ok := legacyReferenceActionBases[normalized]
	if ok {
		return base, true
	}

	if strings.HasPrefix(normalized, "animate") {
		return "the corresponding video model", true
	}
	return "the corresponding image model", true
}

func legacyReferenceActionError(action string) error {
	normalized := normalizedActionName(action)
	base, ok := legacyReferenceActionBase(action)
	if !ok {
		return nil
	}

	kind := "image edit"
	extension := "png"
	if strings.HasPrefix(normalized, "animate") {
		kind = "image-to-video"
		extension = "mp4"
	}

	if strings.HasPrefix(base, "the corresponding ") {
		return fmt.Errorf(
			"%s is a legacy %s action; use %s with --reference instead",
			normalized,
			kind,
			base,
		)
	}

	return fmt.Errorf(
		"%s is a legacy %s action; use the base model with --reference instead, for example: treecli generate %s \"prompt\" --reference @image.png --out output.%s",
		normalized,
		kind,
		base,
		extension,
	)
}
