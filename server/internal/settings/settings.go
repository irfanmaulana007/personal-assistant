// Package settings resolves and persists runtime-editable configuration
// (currently the LLM provider settings), encrypting secrets at rest. The
// database is the single source of truth for these values.
package settings

import (
	"context"
	"fmt"
	"strings"

	"github.com/irfanmaulana007/personal-assistant/server/internal/crypto"
	"github.com/irfanmaulana007/personal-assistant/server/internal/llm"
	"github.com/irfanmaulana007/personal-assistant/server/internal/store"
)

// Setting keys persisted in the store.
const (
	KeyLLMProvider    = "llm.provider"     // plaintext
	KeyLLMAPIKey      = "llm.api_key"      // encrypted
	KeyLLMModel       = "llm.model"        // plaintext
	KeyLLMBaseURL     = "llm.base_url"     // plaintext
	KeyLLMVision      = "llm.vision"       // plaintext "true"/"false"; absent ⇒ false
	KeyComposioAPIKey = "composio.api_key" // encrypted

	KeyWebSearchAPIKey = "websearch.api_key" // encrypted (Tavily Search API key)

	KeyOpenAIAPIKey = "openai.api_key" // encrypted (OpenAI key for image generation)

	KeyRemindersEnabled = "reminders_enabled" // plaintext "true"/"false"; absent ⇒ enabled

	KeyReminderDigestTime  = "reminder_digest_time"  // legacy: local "HH:MM" daily recap (migrated to the start_of_day routine)
	KeyReminderDefaultTime = "reminder_default_time" // local "HH:MM" used when a reminder has no time

	KeyWhatsAppAllowlist = "whatsapp_allowlist" // comma-joined JIDs allowed to chat with the assistant
	KeyWhatsAppAllowAll  = "whatsapp_allow_all" // "true"/"false"; when true the assistant answers every number (allowlist ignored). Absent ⇒ false.

	KeyEvalEnabled    = "eval_enabled"     // "true"/"false"; absent ⇒ enabled
	KeyEvalJudgeModel = "eval_judge_model" // model id for the judge; empty ⇒ reuse the agent model
)

// DefaultReminderTime is used when the user hasn't configured one.
const DefaultReminderTime = "09:00"

// Service resolves and persists settings.
type Service struct {
	store  store.Store
	encKey []byte
}

// New creates a settings service. encKey is the 32-byte AES key (may be nil in
// development, in which case secrets are stored in plaintext).
func New(s store.Store, encKey []byte) *Service {
	return &Service{store: s, encKey: encKey}
}

// LLMConfig resolves the effective LLM configuration from the database. The
// selected provider supplies default base URL/model; explicit stored values
// override those. Falls back to built-in defaults if nothing is set.
func (s *Service) LLMConfig(ctx context.Context) (llm.Config, error) {
	provider, err := s.getString(ctx, KeyLLMProvider)
	if err != nil {
		return llm.Config{}, err
	}
	if provider == "" {
		provider = llm.DefaultProvider
	}
	preset, _ := llm.ProviderByID(provider)

	cfg := llm.Config{
		BaseURL: firstNonEmpty(preset.DefaultBaseURL, llm.DefaultBaseURL),
		Model:   firstNonEmpty(preset.DefaultModel, llm.DefaultModel),
	}

	if v, err := s.getString(ctx, KeyLLMBaseURL); err != nil {
		return cfg, err
	} else if v != "" {
		cfg.BaseURL = v
	}

	if v, err := s.getString(ctx, KeyLLMModel); err != nil {
		return cfg, err
	} else if v != "" {
		cfg.Model = v
	}

	if v, err := s.getString(ctx, KeyLLMVision); err != nil {
		return cfg, err
	} else if v == "true" {
		cfg.Vision = true
	}

	enc, err := s.store.GetSetting(ctx, KeyLLMAPIKey)
	if err != nil {
		return cfg, err
	}
	if len(enc) > 0 {
		dec, err := crypto.Decrypt(s.encKey, enc)
		if err != nil {
			return cfg, fmt.Errorf("decrypt api key: %w", err)
		}
		cfg.APIKey = string(dec)
	}

	return cfg, nil
}

// LLMView is the masked, safe-to-expose view of the LLM settings.
type LLMView struct {
	Provider   string             `json:"provider"`
	Configured bool               `json:"configured"`
	APIKeyMask string             `json:"api_key_mask"`
	Model      string             `json:"model"`
	BaseURL    string             `json:"base_url"`
	Vision     bool               `json:"vision"`
	Providers  []llm.ProviderInfo `json:"providers"`
}

// LLMView returns the current settings with the API key masked, plus the list
// of available providers so the UI can offer a picker.
func (s *Service) LLMView(ctx context.Context) (LLMView, error) {
	cfg, err := s.LLMConfig(ctx)
	if err != nil {
		return LLMView{}, err
	}
	provider, err := s.getString(ctx, KeyLLMProvider)
	if err != nil {
		return LLMView{}, err
	}
	if provider == "" {
		provider = llm.DefaultProvider
	}
	return LLMView{
		Provider:   provider,
		Configured: cfg.APIKey != "",
		APIKeyMask: mask(cfg.APIKey),
		Model:      cfg.Model,
		BaseURL:    cfg.BaseURL,
		Vision:     cfg.Vision,
		Providers:  llm.Providers,
	}, nil
}

// LLMUpdate describes a partial update. A nil field is left unchanged. An empty
// (non-nil) APIKey clears the stored key.
type LLMUpdate struct {
	Provider *string
	APIKey   *string
	Model    *string
	BaseURL  *string
	Vision   *bool
}

// UpdateLLM persists the provided LLM settings.
func (s *Service) UpdateLLM(ctx context.Context, u LLMUpdate) error {
	if u.Provider != nil {
		if err := s.store.SetSetting(ctx, KeyLLMProvider, []byte(*u.Provider)); err != nil {
			return err
		}
	}
	if u.APIKey != nil {
		if *u.APIKey == "" {
			if err := s.store.SetSetting(ctx, KeyLLMAPIKey, []byte{}); err != nil {
				return err
			}
		} else {
			enc, err := crypto.Encrypt(s.encKey, []byte(*u.APIKey))
			if err != nil {
				return fmt.Errorf("encrypt api key: %w", err)
			}
			if err := s.store.SetSetting(ctx, KeyLLMAPIKey, enc); err != nil {
				return err
			}
		}
	}
	if u.Model != nil {
		if err := s.store.SetSetting(ctx, KeyLLMModel, []byte(*u.Model)); err != nil {
			return err
		}
	}
	if u.BaseURL != nil {
		if err := s.store.SetSetting(ctx, KeyLLMBaseURL, []byte(*u.BaseURL)); err != nil {
			return err
		}
	}
	if u.Vision != nil {
		val := "false"
		if *u.Vision {
			val = "true"
		}
		if err := s.store.SetSetting(ctx, KeyLLMVision, []byte(val)); err != nil {
			return err
		}
	}
	return nil
}

// ComposioKey returns the decrypted Composio API key, or "" if unset.
func (s *Service) ComposioKey(ctx context.Context) (string, error) {
	enc, err := s.store.GetSetting(ctx, KeyComposioAPIKey)
	if err != nil {
		return "", err
	}
	if len(enc) == 0 {
		return "", nil
	}
	dec, err := crypto.Decrypt(s.encKey, enc)
	if err != nil {
		return "", fmt.Errorf("decrypt composio key: %w", err)
	}
	return string(dec), nil
}

// SetComposioKey stores the Composio API key encrypted. An empty value clears it.
func (s *Service) SetComposioKey(ctx context.Context, key string) error {
	if key == "" {
		return s.store.SetSetting(ctx, KeyComposioAPIKey, []byte{})
	}
	enc, err := crypto.Encrypt(s.encKey, []byte(key))
	if err != nil {
		return fmt.Errorf("encrypt composio key: %w", err)
	}
	return s.store.SetSetting(ctx, KeyComposioAPIKey, enc)
}

// WebSearchKey returns the decrypted web-search (Tavily) API key, or "" if unset.
func (s *Service) WebSearchKey(ctx context.Context) (string, error) {
	enc, err := s.store.GetSetting(ctx, KeyWebSearchAPIKey)
	if err != nil {
		return "", err
	}
	if len(enc) == 0 {
		return "", nil
	}
	dec, err := crypto.Decrypt(s.encKey, enc)
	if err != nil {
		return "", fmt.Errorf("decrypt web search key: %w", err)
	}
	return string(dec), nil
}

// SetWebSearchKey stores the web-search API key encrypted. An empty value clears it.
func (s *Service) SetWebSearchKey(ctx context.Context, key string) error {
	if key == "" {
		return s.store.SetSetting(ctx, KeyWebSearchAPIKey, []byte{})
	}
	enc, err := crypto.Encrypt(s.encKey, []byte(key))
	if err != nil {
		return fmt.Errorf("encrypt web search key: %w", err)
	}
	return s.store.SetSetting(ctx, KeyWebSearchAPIKey, enc)
}

// OpenAIKey returns the decrypted OpenAI API key (used for image generation),
// or "" if unset.
func (s *Service) OpenAIKey(ctx context.Context) (string, error) {
	enc, err := s.store.GetSetting(ctx, KeyOpenAIAPIKey)
	if err != nil {
		return "", err
	}
	if len(enc) == 0 {
		return "", nil
	}
	dec, err := crypto.Decrypt(s.encKey, enc)
	if err != nil {
		return "", fmt.Errorf("decrypt openai key: %w", err)
	}
	return string(dec), nil
}

// SetOpenAIKey stores the OpenAI API key encrypted. An empty value clears it.
func (s *Service) SetOpenAIKey(ctx context.Context, key string) error {
	if key == "" {
		return s.store.SetSetting(ctx, KeyOpenAIAPIKey, []byte{})
	}
	enc, err := crypto.Encrypt(s.encKey, []byte(key))
	if err != nil {
		return fmt.Errorf("encrypt openai key: %w", err)
	}
	return s.store.SetSetting(ctx, KeyOpenAIAPIKey, enc)
}

// Mask returns a masked view of a secret (e.g. "••••7890"), or "" if empty.
func Mask(secret string) string {
	return mask(secret)
}

// RemindersEnabled reports whether the reminder scheduler is globally enabled.
// The feature is on by default; only an explicit "false" disables it.
func (s *Service) RemindersEnabled(ctx context.Context) bool {
	v, err := s.getString(ctx, KeyRemindersEnabled)
	if err != nil {
		return true
	}
	return v != "false"
}

// SetRemindersEnabled persists the global reminders on/off toggle.
func (s *Service) SetRemindersEnabled(ctx context.Context, enabled bool) error {
	val := "true"
	if !enabled {
		val = "false"
	}
	return s.store.SetSetting(ctx, KeyRemindersEnabled, []byte(val))
}

// ReminderDigestTime returns the legacy daily-recap time ("HH:MM"), or "".
// Retained only so the routine service can migrate it into the start_of_day
// routine; the digest itself has been superseded by daily routines.
func (s *Service) ReminderDigestTime(ctx context.Context) string {
	v, _ := s.getString(ctx, KeyReminderDigestTime)
	return v
}

// SetReminderDigestTime persists the legacy daily-recap time (used to clear it
// once migrated to a routine).
func (s *Service) SetReminderDigestTime(ctx context.Context, hhmm string) error {
	return s.store.SetSetting(ctx, KeyReminderDigestTime, []byte(hhmm))
}

// WhatsAppAllowedJIDs returns the WhatsApp numbers (JIDs) allowed to talk to the
// assistant. The first entry is the primary (receives reminders / the recap).
func (s *Service) WhatsAppAllowedJIDs(ctx context.Context) []string {
	v, _ := s.getString(ctx, KeyWhatsAppAllowlist)
	var out []string
	for _, p := range strings.Split(v, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// SetWhatsAppAllowedJIDs persists the WhatsApp allowlist.
func (s *Service) SetWhatsAppAllowedJIDs(ctx context.Context, jids []string) error {
	return s.store.SetSetting(ctx, KeyWhatsAppAllowlist, []byte(strings.Join(jids, ",")))
}

// WhatsAppAllowAll reports whether the assistant answers every number, ignoring
// the allowlist. Off by default; only an explicit "true" enables it.
func (s *Service) WhatsAppAllowAll(ctx context.Context) bool {
	v, _ := s.getString(ctx, KeyWhatsAppAllowAll)
	return v == "true"
}

// SetWhatsAppAllowAll persists the "answer every number" toggle.
func (s *Service) SetWhatsAppAllowAll(ctx context.Context, allowAll bool) error {
	val := "false"
	if allowAll {
		val = "true"
	}
	return s.store.SetSetting(ctx, KeyWhatsAppAllowAll, []byte(val))
}

// ReminderDefaultTime returns the local "HH:MM" to use for reminders created
// without an explicit time, falling back to DefaultReminderTime.
func (s *Service) ReminderDefaultTime(ctx context.Context) string {
	v, _ := s.getString(ctx, KeyReminderDefaultTime)
	if v == "" {
		return DefaultReminderTime
	}
	return v
}

// SetReminderDefaultTime persists the default reminder time ("HH:MM").
func (s *Service) SetReminderDefaultTime(ctx context.Context, hhmm string) error {
	return s.store.SetSetting(ctx, KeyReminderDefaultTime, []byte(hhmm))
}

// --- Daily routines (scheduled skills) ---

// routineSettingKey composes the persisted key for a routine's field, e.g.
// routine_start_of_day_time.
func routineSettingKey(key, field string) string {
	return "routine_" + key + "_" + field
}

// RoutineField returns the raw stored value for a routine's field, or "" if
// unset (callers supply their own defaults).
func (s *Service) RoutineField(ctx context.Context, key, field string) string {
	v, _ := s.getString(ctx, routineSettingKey(key, field))
	return v
}

// SetRoutineField persists a routine's field value.
func (s *Service) SetRoutineField(ctx context.Context, key, field, value string) error {
	return s.store.SetSetting(ctx, routineSettingKey(key, field), []byte(value))
}

// --- Response evaluation (LLM-as-judge) ---

// EvalEnabled reports whether response scoring is globally enabled. On by
// default; only an explicit "false" disables it.
func (s *Service) EvalEnabled(ctx context.Context) bool {
	v, err := s.getString(ctx, KeyEvalEnabled)
	if err != nil {
		return true
	}
	return v != "false"
}

// SetEvalEnabled persists the global response-scoring on/off toggle.
func (s *Service) SetEvalEnabled(ctx context.Context, enabled bool) error {
	val := "true"
	if !enabled {
		val = "false"
	}
	return s.store.SetSetting(ctx, KeyEvalEnabled, []byte(val))
}

// EvalJudgeModel returns the model id to use for the judge, or "" to reuse the
// configured agent model.
func (s *Service) EvalJudgeModel(ctx context.Context) string {
	v, _ := s.getString(ctx, KeyEvalJudgeModel)
	return v
}

// SetEvalJudgeModel persists the judge model override ("" reuses the agent model).
func (s *Service) SetEvalJudgeModel(ctx context.Context, model string) error {
	return s.store.SetSetting(ctx, KeyEvalJudgeModel, []byte(model))
}

func (s *Service) getString(ctx context.Context, key string) (string, error) {
	v, err := s.store.GetSetting(ctx, key)
	if err != nil {
		return "", err
	}
	return string(v), nil
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

func mask(key string) string {
	if key == "" {
		return ""
	}
	if len(key) <= 4 {
		return "••••"
	}
	return "••••" + key[len(key)-4:]
}
