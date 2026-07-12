// Package imagegen is a minimal HTTP client for OpenAI's Images API using the
// gpt-image-1 model. It supports text-to-image generation and editing an input
// image, returning the raw image bytes (the model always replies with base64).
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
	// model is the image model used for both generation and editing.
	model = "gpt-image-1"
)

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
// /images/edits. gpt-image-1 always returns base64 (never a URL).
type imageResponse struct {
	Data []struct {
		B64JSON string `json:"b64_json"`
	} `json:"data"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
	} `json:"error"`
}

// Generate creates a new image from a text prompt. size and quality are
// optional; empty values let the API pick its defaults.
func (c *Client) Generate(ctx context.Context, apiKey, prompt, size, quality string) (*media.Image, error) {
	body := map[string]any{"model": model, "prompt": prompt, "n": 1}
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
func (c *Client) Edit(ctx context.Context, apiKey, prompt string, input media.Image, size, quality string) (*media.Image, error) {
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	if err := mw.WriteField("model", model); err != nil {
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

func (c *Client) do(req *http.Request) (*media.Image, error) {
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
	return &media.Image{MimeType: "image/png", Data: data}, nil
}
