package whatsapp

import (
	"testing"

	"go.mau.fi/whatsmeow/proto/waE2E"
	"google.golang.org/protobuf/proto"
)

func TestMessageContextInfo(t *testing.T) {
	ci := &waE2E.ContextInfo{Participant: proto.String("123@s.whatsapp.net")}

	// Extended text message carries context info.
	m := &waE2E.Message{ExtendedTextMessage: &waE2E.ExtendedTextMessage{
		Text:        proto.String("hi"),
		ContextInfo: ci,
	}}
	if got := messageContextInfo(m); got != ci {
		t.Errorf("extended text: expected context info, got %v", got)
	}

	// Image message carries context info.
	mImg := &waE2E.Message{ImageMessage: &waE2E.ImageMessage{ContextInfo: ci}}
	if got := messageContextInfo(mImg); got != ci {
		t.Errorf("image: expected context info, got %v", got)
	}

	// Plain conversation has none.
	mPlain := &waE2E.Message{Conversation: proto.String("hi")}
	if got := messageContextInfo(mPlain); got != nil {
		t.Errorf("plain: expected nil context info, got %v", got)
	}
}

func TestContextAddressesSelf(t *testing.T) {
	self := map[string]bool{"5551234": true}

	tests := []struct {
		name string
		ci   *waE2E.ContextInfo
		want bool
	}{
		{"nil context", nil, false},
		{
			"mentioned by phone jid",
			&waE2E.ContextInfo{MentionedJID: []string{"5551234@s.whatsapp.net"}},
			true,
		},
		{
			"mentioned by lid",
			&waE2E.ContextInfo{MentionedJID: []string{"5551234@lid"}},
			true,
		},
		{
			"reply to bot message",
			&waE2E.ContextInfo{Participant: proto.String("5551234@s.whatsapp.net")},
			true,
		},
		{
			"mention of someone else",
			&waE2E.ContextInfo{MentionedJID: []string{"9990000@s.whatsapp.net"}},
			false,
		},
		{
			"reply to someone else",
			&waE2E.ContextInfo{Participant: proto.String("9990000@s.whatsapp.net")},
			false,
		},
		{"empty context", &waE2E.ContextInfo{}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := contextAddressesSelf(tt.ci, self); got != tt.want {
				t.Errorf("contextAddressesSelf() = %v, want %v", got, tt.want)
			}
		})
	}

	// No known self identity → never addressed.
	if contextAddressesSelf(&waE2E.ContextInfo{MentionedJID: []string{"5551234@s.whatsapp.net"}}, nil) {
		t.Error("expected false when self identities are unknown")
	}
}
