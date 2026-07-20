// Package imagegen implements the Image Generator skill: it creates a new image
// from a text prompt, or edits the image the user attached, using OpenAI's
// gpt-image-1 model. The generated image is delivered to the user out of band
// via the media collector on the request context; the string returned to the
// model is a short status line it can narrate.
package imagegen

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/irfanmaulana007/personal-assistant/app/api/internal/imagegen"
	"github.com/irfanmaulana007/personal-assistant/app/api/internal/intent"
	"github.com/irfanmaulana007/personal-assistant/app/api/internal/media"
	"github.com/irfanmaulana007/personal-assistant/app/api/internal/settings"
)

// Handler answers image generation/editing tool calls via the OpenAI client.
type Handler struct {
	client   *imagegen.Client
	settings *settings.Service
	log      *slog.Logger
}

// New creates an image-generation handler.
func New(client *imagegen.Client, settingsSvc *settings.Service, log *slog.Logger) *Handler {
	return &Handler{client: client, settings: settingsSvc, log: log.With("component", "imagegen")}
}

func (h *Handler) Name() string { return "image_generator" }

func (h *Handler) Match(result *intent.ParseResult) bool {
	return result.Capability == intent.CapabilityImageGen
}

func (h *Handler) Handle(ctx context.Context, result *intent.ParseResult) (string, error) {
	prompt := strings.TrimSpace(result.Entities["prompt"])
	if prompt == "" {
		return "What would you like the image to show?", nil
	}

	apiKey, err := h.settings.OpenAIKey(ctx)
	if err != nil {
		return "", fmt.Errorf("resolve openai key: %w", err)
	}
	if apiKey == "" {
		// Reported to the model as text so it can tell the user gracefully.
		return "Image generation is not configured — no OpenAI API key has been set. Ask the user to add an OpenAI API key on the Integrations page.", nil
	}

	size := strings.TrimSpace(result.Entities["size"])
	quality := strings.TrimSpace(result.Entities["quality"])

	var out *imagegen.Result
	switch result.Action {
	case intent.ActionImageEdit:
		input := media.Inbound(ctx)
		if input == nil {
			return "There's no image attached to edit. Ask the user to send the image they want changed.", nil
		}
		out, err = h.client.Edit(ctx, apiKey, prompt, *input, size, quality)
	default: // ActionImageGenerate
		out, err = h.client.Generate(ctx, apiKey, prompt, size, quality)
	}
	if err != nil {
		h.log.Warn("image generation failed", "action", result.Action, "error", err)
		// Surface a readable reason to the model rather than an error page.
		return fmt.Sprintf("Image generation failed: %v", err), nil
	}

	// Deliver the image out of band, and report the OpenAI token usage on the
	// same channel so the agent can track its (much higher) per-image cost
	// separately from the LLM. The API prices InputTokens/OutputTokens with the
	// gpt-image-1 rate, so they map onto prompt/completion respectively.
	if c := media.CollectorFrom(ctx); c != nil && out != nil {
		c.Add(out.Image)
		c.AddUsage(media.ToolUsage{
			Model:            imagegen.Model,
			PromptTokens:     out.Usage.InputTokens,
			CompletionTokens: out.Usage.OutputTokens,
			TotalTokens:      out.Usage.TotalTokens,
		})
	}
	return fmt.Sprintf("The image for %q was created and is attached to your reply. Tell the user it's ready — do not paste any URL or base64 data.", prompt), nil
}
