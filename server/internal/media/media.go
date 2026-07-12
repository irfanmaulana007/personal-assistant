// Package media carries images out of band between the agent, the capability
// handlers, and the response transports. Tool results flow back to the model as
// text, so binary images produced by a tool (e.g. the Image Generator skill)
// can't ride that channel — they are collected here via the request context and
// read back by the agent once the run finishes.
package media

import (
	"context"
	"encoding/base64"
	"strings"
	"sync"
)

// Image is raw image bytes together with their MIME type.
type Image struct {
	MimeType string
	Data     []byte
}

// DataURL encodes the image as a base64 "data:" URL for the web client.
func (img Image) DataURL() string {
	mime := img.MimeType
	if mime == "" {
		mime = "image/png"
	}
	return "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(img.Data)
}

// ParseDataURL decodes a "data:<mime>;base64,<data>" URL into an Image. It
// returns ok=false for anything that isn't a base64 data URL.
func ParseDataURL(s string) (*Image, bool) {
	if !strings.HasPrefix(s, "data:") {
		return nil, false
	}
	comma := strings.IndexByte(s, ',')
	if comma < 0 {
		return nil, false
	}
	header := s[len("data:"):comma]
	if !strings.Contains(header, ";base64") {
		return nil, false
	}
	mime := header[:strings.IndexByte(header, ';')]
	data, err := base64.StdEncoding.DecodeString(s[comma+1:])
	if err != nil {
		return nil, false
	}
	return &Image{MimeType: mime, Data: data}, true
}

type inboundKey struct{}
type collectorKey struct{}

// WithInbound attaches the user's inbound image so image-editing tools can read
// the bytes (the model can't reproduce them as a tool argument).
func WithInbound(ctx context.Context, img *Image) context.Context {
	return context.WithValue(ctx, inboundKey{}, img)
}

// Inbound returns the user's inbound image for this request, or nil if none.
func Inbound(ctx context.Context) *Image {
	img, _ := ctx.Value(inboundKey{}).(*Image)
	return img
}

// Collector accumulates images produced by tools during a single agent run. It
// is safe for concurrent use (tool calls in one step may run in sequence today,
// but the mutex keeps this correct regardless).
type Collector struct {
	mu     sync.Mutex
	images []Image
}

// Add records a produced image.
func (c *Collector) Add(img Image) {
	c.mu.Lock()
	c.images = append(c.images, img)
	c.mu.Unlock()
}

// Images returns a copy of the images collected so far.
func (c *Collector) Images() []Image {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]Image, len(c.images))
	copy(out, c.images)
	return out
}

// WithCollector attaches a fresh Collector to the context and returns both.
func WithCollector(ctx context.Context) (context.Context, *Collector) {
	c := &Collector{}
	return context.WithValue(ctx, collectorKey{}, c), c
}

// CollectorFrom returns the Collector in the context, or nil if absent. Handlers
// should nil-check before adding so they stay usable outside an agent run.
func CollectorFrom(ctx context.Context) *Collector {
	c, _ := ctx.Value(collectorKey{}).(*Collector)
	return c
}
