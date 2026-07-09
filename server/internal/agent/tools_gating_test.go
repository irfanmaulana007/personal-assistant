package agent

import "testing"

func TestReminderToolsAreAlwaysOn(t *testing.T) {
	base := make(map[string]bool)
	for _, tl := range toolSchemas() {
		base[tl.Function.Name] = true
	}
	for _, n := range []string{"reminder_set", "reminder_schedule", "reminder_list", "reminder_cancel"} {
		if !base[n] {
			t.Errorf("%s should be an always-on base tool", n)
		}
		if _, ok := toolByName[n]; !ok {
			t.Errorf("%s must be routable via toolByName", n)
		}
	}

	// No skills enabled → no skill tools exposed (reminders no longer depend on one).
	if len(skillToolSchemas(nil)) != 0 {
		t.Error("no skill tools should be exposed when no skills are enabled")
	}
}
