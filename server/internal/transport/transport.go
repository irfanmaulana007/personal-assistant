package transport

import "context"

// Message represents a platform-agnostic incoming message.
type Message struct {
	ID        string
	From      string
	Text      string
	Platform  string
	Timestamp int64
	Raw       any
}

// MessageHandler is called when a message is received.
type MessageHandler func(msg *Message)

// SendFunc sends a text reply to a recipient on a transport.
type SendFunc func(ctx context.Context, to, text string) error

// Transport defines the interface for messaging platforms.
type Transport interface {
	// Name returns the transport identifier (e.g., "whatsapp").
	Name() string

	// Start connects to the platform and begins listening for messages.
	Start(ctx context.Context) error

	// Stop gracefully disconnects from the platform.
	Stop() error

	// SetMessageHandler registers the callback for incoming messages.
	SetMessageHandler(handler MessageHandler)

	// SendMessage sends a text message to the given recipient.
	SendMessage(ctx context.Context, to, text string) error
}
