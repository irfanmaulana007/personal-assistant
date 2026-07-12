package agent

import "testing"

func TestHasSaveIntent(t *testing.T) {
	// Turns that ask the assistant to persist something must force a tool call.
	saveIntent := []string{
		"Catet ini ke learning list", // run #60 — the fabricated-confirmation case
		"catat ini ke bucket list",
		"tambahkan meeting jam 3",
		"tambahin ke daftar",
		"masukin ke bucket list",
		"tolong ingatkan aku besok jam 9",
		"ingetin aku meeting nanti sore",
		"jadwalkan meeting besok",
		"simpan catatan ini",
		"add visit Japan to my bucket list",
		"save this note",
		"remind me tomorrow at 9am",
		"set a reminder for the meeting",
		"schedule a call for Friday",
	}
	for _, msg := range saveIntent {
		if !hasSaveIntent(msg) {
			t.Errorf("expected save intent for %q", msg)
		}
	}

	// Conversational / read-only turns must NOT force a tool call, or the model
	// would be forced to call an irrelevant tool.
	noSaveIntent := []string{
		"makasih ya",
		"terima kasih banyak",
		"apa ibukota Prancis?",
		"gimana kabarmu?",
		"what's the weather like?",
		"who won the world cup?",
		"halo",
	}
	for _, msg := range noSaveIntent {
		if hasSaveIntent(msg) {
			t.Errorf("did not expect save intent for %q", msg)
		}
	}
}
