package main

import (
	"testing"

	"github.com/irfanmaulana007/personal-assistant/app/api/internal/transport"
)

func TestSenderPhone(t *testing.T) {
	tests := []struct {
		name string
		msg  *transport.Message
		want string
	}{
		{
			"phone-form From",
			&transport.Message{From: "628123@s.whatsapp.net", Candidates: []string{"628123@s.whatsapp.net"}},
			"+628123",
		},
		{
			"lid From but phone candidate present",
			&transport.Message{
				From:       "111222@lid",
				Candidates: []string{"111222@lid", "628123@s.whatsapp.net"},
			},
			"+628123",
		},
		{
			"lid-only sender, no phone resolved",
			&transport.Message{From: "111222@lid", Candidates: []string{"111222@lid"}},
			"",
		},
		{
			"no candidates",
			&transport.Message{From: "628123@s.whatsapp.net"},
			"",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := senderPhone(tt.msg); got != tt.want {
				t.Errorf("senderPhone() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestGroupSenderLabel(t *testing.T) {
	tests := []struct {
		name string
		msg  *transport.Message
		want string
	}{
		{
			"name and phone",
			&transport.Message{
				SenderName: "Budi Santoso",
				From:       "628123@s.whatsapp.net",
				Candidates: []string{"628123@s.whatsapp.net"},
			},
			"Budi Santoso (+628123)",
		},
		{
			"name only (lid-only sender)",
			&transport.Message{
				SenderName: "Budi Santoso",
				From:       "111222@lid",
				Candidates: []string{"111222@lid"},
			},
			"Budi Santoso",
		},
		{
			"phone only (no display name)",
			&transport.Message{
				From:       "628123@s.whatsapp.net",
				Candidates: []string{"628123@s.whatsapp.net"},
			},
			"+628123",
		},
		{
			"neither name nor phone falls back to raw JID",
			&transport.Message{From: "111222@lid", Candidates: []string{"111222@lid"}},
			"111222@lid",
		},
		{
			"whitespace-only display name is ignored",
			&transport.Message{
				SenderName: "   ",
				From:       "628123@s.whatsapp.net",
				Candidates: []string{"628123@s.whatsapp.net"},
			},
			"+628123",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := groupSenderLabel(tt.msg); got != tt.want {
				t.Errorf("groupSenderLabel() = %q, want %q", got, tt.want)
			}
		})
	}
}
