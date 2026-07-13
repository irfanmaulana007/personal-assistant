package store

// skillSeed is the master list of skills, owned by code and upserted on boot.
// prunedSkillKeys are skills removed from the product; their rows (and any
// per-user toggles) are deleted on boot. Reminders became a core, always-on
// capability, so its skill toggle is gone. life_goals was replaced by
// bucket_list: the retired row (which still carried the old prompt telling the
// model to call the non-existent lifegoal_add tool) is dropped so only
// bucket_list remains.
var prunedSkillKeys = []string{"scheduled_reminder", "life_goals"}

// DefaultSkillPrompt returns the code-owned default prompt for a skill key, or
// "" if the key is unknown. Used to reset an admin-customized prompt back to
// the shipped default.
func DefaultSkillPrompt(key string) string {
	for _, sk := range skillSeed {
		if sk.Key == key {
			return sk.Prompt
		}
	}
	return ""
}

var skillSeed = []Skill{
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
		Key:            "bucket_list",
		Name:           "Bucket List",
		Category:       "Personal",
		DefaultEnabled: true,
		SortOrder:      1,
		Description:    "Keep a bucket list of things you want to do in life — \"take a swimming course\", \"visit Japan\", \"climb Rinjani\" — sorted into categories. Add them just by mentioning them, ask to see your list, and check them off as you go. Also manageable from the Bucket List page, where you can flag items as this year's resolutions.",
		Prompt:         "The user keeps a bucket list — a categorized checklist of things they want to do in life. Use bucketlist_add when they mention wanting to do or achieve something someday, inferring the category (self_improvement, learning, hiking, country, local, other). Use bucketlist_list to show the list and its progress, bucketlist_check to mark an item done when they've achieved it, and bucketlist_delete to remove one. Identify an item to check or delete by its number from the last listing or by its title. You MUST call bucketlist_add (or bucketlist_check) to change the list — only confirm an item after the tool call returns successfully, and never claim you added or checked off something without actually calling the tool. Be encouraging when they complete something.",
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
		DefaultEnabled: true,
		SortOrder:      7,
		Description:    "Practice English while you chat. When you write in English, the assistant shows a corrected version of your message (grammar/spelling fixed), then answers normally. Messages in other languages are answered normally.",
		Prompt:         "The user is actively practicing English, and correcting their English is a TOP priority on every turn. On EVERY message the user writes in English — without exception, including short messages, greetings, one-word replies, and follow-ups, and even when you also call a tool — you MUST begin your response with the grammatically corrected version of their message, wrapped exactly between the markers [[grammar]] and [[/grammar]]. Inside those markers put ONLY the corrected sentence: fix grammar, spelling, articles, tense, prepositions, and word choice while keeping their meaning and tone — no explanation, labels, or commentary. If their message is already correct, repeat it unchanged inside the markers. Immediately after the closing [[/grammar]] marker, answer their message normally (using tools/actions as usual), replying in English. If the user writes in a language other than English, do NOT output the [[grammar]] block at all — simply answer normally in that same language. Never skip the [[grammar]] block for an English message, even when the reply is very short or you are calling a tool.",
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
		Key:            "web_search",
		Name:           "Web Search",
		Category:       "Knowledge",
		DefaultEnabled: false,
		SortOrder:      8,
		Description:    "Let the assistant look things up on the open web — news, sports scores, prices, weather, or anything more recent than its training data. Ask a question about the wider world and it searches, then answers with sources. Requires a Tavily API key (set it on the Integrations page).",
		Prompt:         "You can search the open web with the web_search tool. Use it whenever the user asks about current or real-world information you don't already have — news, sports scores and brackets, prices, weather, recent events, or any fact that may be newer than your training cutoff — instead of saying you lack real-time access. Search first, then answer from the results: summarize concisely in the user's language and cite the source links. Never invent facts beyond what the results contain; if the search returns nothing useful or web search isn't configured, say so plainly. Do not use it for the user's own private data (calendar, email, notes, contacts) — those have their own tools.",
	},
	{
		Key:            "image_generator",
		Name:           "Image Generator",
		Category:       "Creative",
		DefaultEnabled: false,
		SortOrder:      9,
		Description:    "Create images from a description, or edit a photo you send. Ask the assistant to \"draw a cat astronaut\" and it generates a picture; attach a photo and say \"make the sky purple\" and it edits it. Requires an OpenAI API key (set it on the Integrations page).",
		Prompt:         "You can create and edit images. Use the generate_image tool when the user asks you to draw, create, generate, design, or imagine any picture, illustration, logo, or artwork — write a rich, detailed English prompt describing the subject, style, composition, and colours. Use the edit_image tool when the user has attached an image and asks you to change it (recolour, add or remove something, restyle). The finished image is delivered to the user automatically, so never paste base64 or image URLs into your reply — just briefly describe what you made in the user's language. If image generation isn't configured, say so plainly.",
	},
	{
		Key:            "translator",
		Name:           "Translator",
		Category:       "Communication",
		DefaultEnabled: false,
		SortOrder:      10,
		Description:    "Chat across two languages in a WhatsApp group. In a group, set the pair once with \"/t set Indonesian Japanese\", then mention the assistant with \"/t <message>\" and it replies with the translation — so you and a foreign friend can each write in your own language and still understand each other. It auto-detects which of the two languages you wrote in and translates to the other. By default it shows just the translation; use \"/t mode both\" to also show the original. Set the tone with \"/t formality casual\" or \"formal\". Grammar is always corrected in the translation.",
		// No system-prompt fragment: the `/t` command is handled deterministically
		// in the WhatsApp group path (see internal/translate.GroupService), so this
		// skill only needs its on/off toggle and description — enabling it does not
		// change the general assistant's behaviour.
		Prompt: "",
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
	{
		Key:            "self_tuning",
		Name:           "Self-Tuning",
		Category:       "System",
		DefaultEnabled: false,
		SortOrder:      11,
		Description:    "Let the assistant improve its own skills. When on, it can review recent low-quality conversations (where a skill was involved) and rewrite that skill's instructions so it does better next time. Meant to run from the End of day routine; the improved prompt persists and can be reverted from this page.",
		Prompt:         "You can review your own recent low-quality conversations and refine the instruction prompts of your other skills so they work better next time. Use review_skill_performance to pull recent low-scoring conversations that involved a skill, together with each involved skill's current prompt. Study each conversation's input, your output, the tools you called, and the judge's rationale to diagnose WHY the skill underperformed (missing instruction, ambiguous guidance, a tool it should have called but didn't, wrong format). Then use update_skill_prompt to save an improved prompt for that skill: keep everything that already works, make a focused, surgical change that fixes the observed failure, preserve the tool names and any required output markers exactly, and pass a one-line reason. Do not rewrite a prompt wholesale, invent skills that don't exist, or tune a skill you have no evidence is failing. Never touch the self_tuning skill itself.",
	},
}
