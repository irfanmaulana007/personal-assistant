package agent

import "testing"

func TestHasLookupIntent(t *testing.T) {
	// Turns that ask to read back integration data must force a tool call so the
	// answer is grounded in a real tool result, not fabricated.
	lookupIntent := []string{
		"ada trello yg connect ga?", // run #276 — the fabricated-boards case
		"trello ku ada board apa aja?",
		"list my trello boards",
		"apa aja card di board ini?",
		"kartu apa aja yang belum selesai?",
		"cek inbox dong",
		"any new emails?",
		"reminder aku apa aja hari ini?",
		"pengingat besok apa aja?",
		"agenda ku minggu ini apa?",
		"jadwal aku hari ini gimana?",
		"siapa aja kontak yang tersimpan?",
		"what's on my bucket list?",
	}
	for _, msg := range lookupIntent {
		if !hasLookupIntent(msg) {
			t.Errorf("expected lookup intent for %q", msg)
		}
	}

	// Conversational turns must NOT force a tool call, or the model would be
	// forced to call an irrelevant tool.
	noLookupIntent := []string{
		"makasih ya",
		"terima kasih banyak",
		"apa ibukota Prancis?",
		"gimana kabarmu?",
		"what's the weather like?",
		"halo",
	}
	for _, msg := range noLookupIntent {
		if hasLookupIntent(msg) {
			t.Errorf("did not expect lookup intent for %q", msg)
		}
	}
}
