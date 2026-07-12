// Package imagegen is a minimal HTTP client for OpenAI's Images API using the
// gpt-image-1-mini model (the cheaper sibling of gpt-image-1). It supports
// text-to-image generation and editing an input image, returning the raw image
// bytes (the model always replies with base64) together with the token usage
// the API reports for cost tracking.
package imagegen

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"strings"
	"time"

	"github.com/irfanmaulana007/personal-assistant/server/internal/media"
)

const (
	// baseURL is the OpenAI Images API host.
	baseURL = "https://api.openai.com/v1"
	// Model is the image model used for both generation and editing. It doubles
	// as the pricing key so cost estimates line up with the LLM price table.
	// gpt-image-1-mini is the low-cost variant of gpt-image-1.
	Model = "gpt-image-1-mini"
)

// Usage is the token usage gpt-image-1 reports for a single request. The Images
// API bills per token: InputTokens covers the text prompt (and, for edits, the
// input image), OutputTokens covers the generated image. Callers price it via
// the model's InputPer1M/OutputPer1M rate.
type Usage struct {
	InputTokens  int
	OutputTokens int
	TotalTokens  int
}

// Result is a generated/edited image plus the usage the API reported for it.
type Result struct {
	Image media.Image
	Usage Usage
}

// Client calls the OpenAI Images API.
type Client struct {
	http    *http.Client
	baseURL string
}

// NewClient returns a client with a generous timeout — image generation can
// take tens of seconds.
func NewClient() *Client {
	return &Client{http: &http.Client{Timeout: 120 * time.Second}, baseURL: baseURL}
}

// imageResponse is the shape returned by both /images/generations and
// /images/edits. gpt-image-1 always returns base64 (never a URL) and includes a
// usage object with the token counts used for cost tracking.
type imageResponse struct {
	Data []struct {
		B64JSON string `json:"b64_json"`
	} `json:"data"`
	Usage *struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
		TotalTokens  int `json:"total_tokens"`
	} `json:"usage"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
	} `json:"error"`
}

// Generate creates a new image from a text prompt. size and quality are
// optional; empty values let the API pick its defaults.
func (c *Client) Generate(ctx context.Context, apiKey, prompt, size, quality string) (*Result, error) {
	body := map[string]any{"model": Model, "prompt": prompt, "n": 1}
	if size != "" {
		body["size"] = size
	}
	if quality != "" {
		body["quality"] = quality
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/images/generations", bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)
	return c.do(req)
}

// Edit produces a new image from an input image and an edit instruction.
func (c *Client) Edit(ctx context.Context, apiKey, prompt string, input media.Image, size, quality string) (*Result, error) {
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	if err := mw.WriteField("model", Model); err != nil {
		return nil, err
	}
	if err := mw.WriteField("prompt", prompt); err != nil {
		return nil, err
	}
	if size != "" {
		if err := mw.WriteField("size", size); err != nil {
			return nil, err
		}
	}
	if quality != "" {
		if err := mw.WriteField("quality", quality); err != nil {
			return nil, err
		}
	}

	part, err := mw.CreatePart(imagePartHeader(input.MimeType))
	if err != nil {
		return nil, fmt.Errorf("create image part: %w", err)
	}
	if _, err := part.Write(input.Data); err != nil {
		return nil, fmt.Errorf("write image part: %w", err)
	}
	if err := mw.Close(); err != nil {
		return nil, fmt.Errorf("close multipart: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/images/edits", &buf)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+apiKey)
	return c.do(req)
}

// imagePartHeader builds the multipart header for the "image" file field with a
// filename/extension matching the MIME type (the API infers format from it).
func imagePartHeader(mime string) textproto.MIMEHeader {
	ext := "png"
	switch mime {
	case "image/jpeg", "image/jpg":
		ext = "jpg"
	case "image/webp":
		ext = "webp"
	}
	if mime == "" {
		mime = "image/png"
	}
	h := textproto.MIMEHeader{}
	h.Set("Content-Disposition", fmt.Sprintf(`form-data; name="image"; filename="image.%s"`, ext))
	h.Set("Content-Type", mime)
	return h
}

func (c *Client) do(req *http.Request) (*Result, error) {
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("call image api: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	var parsed imageResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, fmt.Errorf("parse response (status %d): %w", resp.StatusCode, err)
	}
	if resp.StatusCode != http.StatusOK {
		if parsed.Error != nil {
			return nil, fmt.Errorf("image api error (status %d): %s", resp.StatusCode, parsed.Error.Message)
		}
		return nil, fmt.Errorf("image api error (status %d): %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	if len(parsed.Data) == 0 || parsed.Data[0].B64JSON == "" {
		return nil, fmt.Errorf("image api returned no image")
	}

	data, err := base64.StdEncoding.DecodeString(parsed.Data[0].B64JSON)
	if err != nil {
		return nil, fmt.Errorf("decode image: %w", err)
	}
	res := &Result{Image: media.Image{MimeType: "image/png", Data: data}}
	if parsed.Usage != nil {
		res.Usage = Usage{
			InputTokens:  parsed.Usage.InputTokens,
			OutputTokens: parsed.Usage.OutputTokens,
			TotalTokens:  parsed.Usage.TotalTokens,
		}
	}
	return res, nil
}
