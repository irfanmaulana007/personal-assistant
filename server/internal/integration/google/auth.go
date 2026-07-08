package google

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/calendar/v3"
	"google.golang.org/api/gmail/v1"

	"github.com/irfanmaulana007/personal-assistant/server/internal/crypto"
	"github.com/irfanmaulana007/personal-assistant/server/internal/store"
)

// Auth manages Google OAuth2 authentication with encrypted token storage.
type Auth struct {
	config        *oauth2.Config
	store         store.Store
	encryptionKey []byte
	log           *slog.Logger
}

// NewAuth creates a new Google OAuth2 auth manager.
func NewAuth(credentialsFile string, s store.Store, encryptionKey string, log *slog.Logger) (*Auth, error) {
	data, err := os.ReadFile(credentialsFile)
	if err != nil {
		return nil, fmt.Errorf("read credentials file: %w", err)
	}

	config, err := google.ConfigFromJSON(data,
		calendar.CalendarReadonlyScope,
		calendar.CalendarEventsScope,
		gmail.GmailReadonlyScope,
		gmail.GmailComposeScope,
		gmail.GmailLabelsScope,
	)
	if err != nil {
		return nil, fmt.Errorf("parse credentials: %w", err)
	}

	key, err := crypto.DecodeKey(encryptionKey)
	if err != nil {
		return nil, err
	}

	return &Auth{
		config:        config,
		store:         s,
		encryptionKey: key,
		log:           log.With("component", "google-auth"),
	}, nil
}

// GetToken retrieves a valid OAuth2 token, prompting for authorization if needed.
func (a *Auth) GetToken(ctx context.Context) (*oauth2.Token, error) {
	// Try loading from store
	tokenData, err := a.store.GetToken(ctx, "google")
	if err != nil {
		return nil, fmt.Errorf("load token: %w", err)
	}

	if tokenData != nil {
		decrypted, err := crypto.Decrypt(a.encryptionKey, tokenData)
		if err != nil {
			a.log.Warn("failed to decrypt token, re-authorizing", "error", err)
		} else {
			var token oauth2.Token
			if err := json.Unmarshal(decrypted, &token); err != nil {
				a.log.Warn("failed to parse token, re-authorizing", "error", err)
			} else {
				// Token source will auto-refresh if expired
				ts := a.config.TokenSource(ctx, &token)
				newToken, err := ts.Token()
				if err != nil {
					a.log.Warn("token refresh failed, re-authorizing", "error", err)
				} else {
					// Save if token was refreshed
					if newToken.AccessToken != token.AccessToken {
						if err := a.saveToken(ctx, newToken); err != nil {
							a.log.Error("failed to save refreshed token", "error", err)
						}
					}
					return newToken, nil
				}
			}
		}
	}

	// Need to authorize
	return a.authorize(ctx)
}

func (a *Auth) authorize(ctx context.Context) (*oauth2.Token, error) {
	authURL := a.config.AuthCodeURL("state-token", oauth2.AccessTypeOffline)
	fmt.Printf("\n=== Google Authorization ===\n")
	fmt.Printf("Visit this URL to authorize:\n%s\n\n", authURL)
	fmt.Print("Enter the authorization code: ")

	var code string
	if _, err := fmt.Scan(&code); err != nil {
		return nil, fmt.Errorf("read auth code: %w", err)
	}

	token, err := a.config.Exchange(ctx, code)
	if err != nil {
		return nil, fmt.Errorf("exchange code: %w", err)
	}

	if err := a.saveToken(ctx, token); err != nil {
		return nil, fmt.Errorf("save token: %w", err)
	}

	a.log.Info("Google authorization successful")
	return token, nil
}

func (a *Auth) saveToken(ctx context.Context, token *oauth2.Token) error {
	data, err := json.Marshal(token)
	if err != nil {
		return fmt.Errorf("marshal token: %w", err)
	}

	encrypted, err := crypto.Encrypt(a.encryptionKey, data)
	if err != nil {
		return fmt.Errorf("encrypt token: %w", err)
	}

	return a.store.SaveToken(ctx, "google", encrypted)
}

// TokenSource returns an oauth2.TokenSource for creating API clients.
func (a *Auth) TokenSource(ctx context.Context) (oauth2.TokenSource, error) {
	token, err := a.GetToken(ctx)
	if err != nil {
		return nil, err
	}
	return a.config.TokenSource(ctx, token), nil
}
