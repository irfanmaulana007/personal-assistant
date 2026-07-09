package agent

import "testing"

func TestReminderToolsGatedBySkill(t *testing.T) {
	base := make(map[string]bool)
	for _, tl := range toolSchemas() {
		base[tl.Function.Name] = true
	}
	for _, n := range []string{"reminder_set", "reminder_list", "reminder_cancel"} {
		if base[n] {
			t.Errorf("%s should not be in the always-on base tools", n)
		}
	}

	gated := make(map[string]bool)
	for _, tl := range skillToolSchemas([]string{"scheduled_reminder"}) {
		gated[tl.Function.Name] = true
	}
	for _, n := range []string{"reminder_set", "reminder_list", "reminder_cancel"} {
		if !gated[n] {
			t.Errorf("%s should be provided by the scheduled_reminder skill", n)
		}
		if _, ok := toolByName[n]; !ok {
			t.Errorf("%s must remain routable via toolByName", n)
		}
	}

	// No skills enabled → no skill tools exposed.
	if len(skillToolSchemas(nil)) != 0 {
		t.Error("no skill tools should be exposed when no skills are enabled")
	}
}
