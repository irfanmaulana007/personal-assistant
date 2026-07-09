// Package settings resolves and persists runtime-editable configuration
// (currently the LLM provider settings), encrypting secrets at rest. The
// database is the single source of truth for these values.
package settings

import (
	"context"
	"fmt"

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
	KeyComposioAPIKey = "composio.api_key" // encrypted

	KeyRemindersEnabled = "reminders_enabled" // plaintext "true"/"false"; absent ⇒ enabled
)

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
