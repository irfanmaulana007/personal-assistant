package store

import (
	"context"
	"database/sql"
	"fmt"
)

// skillSeed is the master list of skills, owned by code and upserted on boot.
var skillSeed = []Skill{
	{
		Key:            "scheduled_reminder",
		Name:           "Scheduled Reminder",
		Category:       "Productivity",
		DefaultEnabled: true,
		SortOrder:      1,
		Description:    "Set reminders that reach you on WhatsApp — from one-off nudges to bills and appointments. Tell the assistant what to remember and when, and it pings your WhatsApp when it's due.",
		Prompt:         "You can set, list, and cancel reminders for the user; due reminders are delivered to their WhatsApp. The user's reminders ARE their schedule/calendar/agenda — when they ask what's on their schedule or calendar, what's coming up, or what they have planned today/tomorrow/this week, call reminder_list and answer from it (do not say you lack calendar access). When an item shows an event time, present that actual event time as when it happens — not the earlier reminder/notification time. Capture a clear message and a specific time when setting one. Always confirm the exact message and date/time you scheduled.",
	},
	{
		Key:            "ask_about_contact",
		Name:           "Ask About Contact",
		Category:       "Personal",
		DefaultEnabled: false,
		SortOrder:      2,
		Description:    "Save and look up your contacts. Tell the assistant to remember someone's phone, email, or a note, then just ask \"what's John's number?\" whenever you need it.",
		Prompt:         "You can look up saved contacts with the contact_search tool and save new ones with contact_add. When the user asks about a person (their phone, email, or a note), search for them. When the user shares contact details (\"save John's number 0812…\", \"Sarah's email is …\"), save them. Always confirm what you found or saved, and never invent contact details.",
	},
	{
		Key:            "travel_control",
		Name:           "Travel Control",
		Category:       "Finance",
		DefaultEnabled: false,
		SortOrder:      3,
		Description:    "Track spending on a trip. Start a trip, log expenses as you go (\"paid 200k for the hotel\"), and get a running total by category and what's left of your budget — optionally synced to Google Sheets.",
		Prompt:         "You track trip expenses. Use trip_create to start a named trip (optional destination and budget), expense_add to record an expense (amount, currency, category, note) against the active or named trip, and trip_summary to report totals by category and remaining budget. When the user mentions spending money on a trip, record it and confirm the amount and category. If the user has connected Google Sheets, you may also append the expense to their sheet.",
	},
	{
		Key:            "activity_summary",
		Name:           "Sports & Workout Summary",
		Category:       "Health",
		DefaultEnabled: false,
		SortOrder:      4,
		Description:    "Keep a log of your sports and workouts and get a recap. Mention a session (\"ran 5k this morning\") and the assistant logs it; ask \"how did I train this week?\" for a summary.",
		Prompt:         "You track the user's sports and workouts. Use activity_log to record an activity they mention (type, a short description, and when it happened), and activity_summarize to report their recent activity over a period (sessions by type, totals, and trends). You may also surface workout-related reminders. If the period is unclear, ask. Keep summaries concise and encouraging.",
	},
	{
		Key:            "english_tutor",
		Name:           "English Tutor",
		Category:       "Learning",
		DefaultEnabled: false,
		SortOrder:      7,
		Description:    "Practice English while you chat. When you write in English, the assistant shows a corrected version of your message (grammar/spelling fixed), then answers normally. Messages in other languages are answered normally.",
		Prompt:         "The user is practicing English. When the user's message is written in English, begin your response with ONLY the grammatically corrected version of their message — fix grammar, spelling, articles, tense, prepositions, and word choice, but keep their meaning and tone. Output nothing else in this part: no explanation, no labels, no commentary. Wrap it exactly between the markers [[grammar]] and [[/grammar]]. If their message is already correct, put the message unchanged inside the markers. After the closing [[/grammar]] marker, answer their message normally (using tools/actions as usual). If the user writes in a language other than English, do NOT include the [[grammar]] block at all — just answer normally.",
	},
	{
		Key:            "hiking_tracker",
		Name:           "Hiking Tracker",
		Category:       "Outdoors",
		DefaultEnabled: false,
		SortOrder:      6,
		Description:    "Log your hikes in detail — the mountain, the trails you took up and down, whether you camped, how many days and nights, the date, and who came along. The assistant reuses your existing mountain, trail, and friend names so a small typo never creates a duplicate.",
		Prompt:         "You keep a detailed log of the user's hiking trips. Use hike_log to record a hike, capturing: the mountain/destination, the trail used going up, the trail used going down, whether they camped (yes/no), how many days and how many nights, the hiking date, and the participants (as a comma-separated list). Use hike_summary to review past hikes. The system automatically matches similar existing mountain, trail, and participant names to prevent duplicates from typos, so pass names as the user says them and mention when it reused an existing name. If the mountain or date is missing, ask one short question before logging.",
	},
	{
		Key:            "food_calories",
		Name:           "Food Calories",
		Category:       "Health",
		DefaultEnabled: false,
		SortOrder:      5,
		Description:    "Estimate the calories in a meal from a photo. Send a picture of your food and the assistant identifies the items and gives an approximate calorie count and macros. Needs a vision-capable model.",
		Prompt:         "The user may send a photo of a meal. Identify the food items, estimate the portion sizes, and give an approximate per-item and total calorie count plus a rough protein/carbs/fat breakdown — always clearly labelled as estimates that vary with portion and preparation. If the user only describes a meal in text, estimate from the description. This needs a vision-capable model; if you cannot see the image, say so.",
	},
}

func (s *SQLiteStore) seedSkills() error {
	for _, sk := range skillSeed {
		if _, err := s.db.Exec(
			`INSERT INTO skills (key, name, description, prompt, category, default_enabled, sort_order)
			 VALUES (?, ?, ?, ?, ?, ?, ?)
			 ON CONFLICT(key) DO UPDATE SET
			   name = excluded.name,
			   description = excluded.description,
			   prompt = excluded.prompt,
			   category = excluded.category,
			   default_enabled = excluded.default_enabled,
			   sort_order = excluded.sort_order`,
			sk.Key, sk.Name, sk.Description, sk.Prompt, sk.Category, boolToInt(sk.DefaultEnabled), sk.SortOrder,
		); err != nil {
			return fmt.Errorf("seed skill %s: %w", sk.Key, err)
		}
	}
	return nil
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// --- Skills ---

func (s *SQLiteStore) ListSkills(ctx context.Context) ([]Skill, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, key, name, description, prompt, category, default_enabled, sort_order
		 FROM skills ORDER BY sort_order ASC, id ASC`)
	if err != nil {
		return nil, fmt.Errorf("list skills: %w", err)
	}
	defer rows.Close()
	return scanSkills(rows)
}

func (s *SQLiteStore) GetSkill(ctx context.Context, id int64) (*Skill, error) {
	var sk Skill
	var de int
	err := s.db.QueryRowContext(ctx,
		`SELECT id, key, name, description, prompt, category, default_enabled, sort_order FROM skills WHERE id = ?`, id,
	).Scan(&sk.ID, &sk.Key, &sk.Name, &sk.Description, &sk.Prompt, &sk.Category, &de, &sk.SortOrder)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get skill: %w", err)
	}
	sk.DefaultEnabled = de != 0
	return &sk, nil
}

// ListUserSkills returns all skills with the effective enabled state for a user
// (the user's override if present, otherwise the skill's default).
func (s *SQLiteStore) ListUserSkills(ctx context.Context, userID int64) ([]UserSkill, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT s.id, s.key, s.name, s.description, s.prompt, s.category, s.default_enabled, s.sort_order,
		        COALESCE(us.enabled, s.default_enabled) AS effective
		 FROM skills s
		 LEFT JOIN user_skills us ON us.skill_id = s.id AND us.user_id = ?
		 ORDER BY s.sort_order ASC, s.id ASC`, userID)
	if err != nil {
		return nil, fmt.Errorf("list user skills: %w", err)
	}
	defer rows.Close()

	var out []UserSkill
	for rows.Next() {
		var us UserSkill
		var de, eff int
		if err := rows.Scan(&us.ID, &us.Key, &us.Name, &us.Description, &us.Prompt, &us.Category, &de, &us.SortOrder, &eff); err != nil {
			return nil, fmt.Errorf("scan user skill: %w", err)
		}
		us.DefaultEnabled = de != 0
		us.Enabled = eff != 0
		out = append(out, us)
	}
	return out, rows.Err()
}

func (s *SQLiteStore) SetSkillEnabled(ctx context.Context, userID, skillID int64, enabled bool) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO user_skills (user_id, skill_id, enabled) VALUES (?, ?, ?)
		 ON CONFLICT(user_id, skill_id) DO UPDATE SET enabled = excluded.enabled`,
		userID, skillID, boolToInt(enabled),
	)
	return err
}

func (s *SQLiteStore) EnabledSkillKeys(ctx context.Context, userID int64) ([]string, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT s.key FROM skills s
		 LEFT JOIN user_skills us ON us.skill_id = s.id AND us.user_id = ?
		 WHERE COALESCE(us.enabled, s.default_enabled) = 1`, userID)
	if err != nil {
		return nil, fmt.Errorf("enabled skill keys: %w", err)
	}
	defer rows.Close()

	var keys []string
	for rows.Next() {
		var k string
		if err := rows.Scan(&k); err != nil {
			return nil, err
		}
		keys = append(keys, k)
	}
	return keys, rows.Err()
}

func scanSkills(rows *sql.Rows) ([]Skill, error) {
	var out []Skill
	for rows.Next() {
		var sk Skill
		var de int
		if err := rows.Scan(&sk.ID, &sk.Key, &sk.Name, &sk.Description, &sk.Prompt, &sk.Category, &de, &sk.SortOrder); err != nil {
			return nil, fmt.Errorf("scan skill: %w", err)
		}
		sk.DefaultEnabled = de != 0
		out = append(out, sk)
	}
	return out, rows.Err()
}
