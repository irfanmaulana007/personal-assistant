// Package mcpoauth runs the OAuth 2.1 authorization-code flow for MCP servers
// whose hosted endpoints require OAuth (Notion, Railway). It performs discovery
// (RFC 9728 protected-resource metadata → RFC 8414 authorization-server
// metadata), dynamic client registration (RFC 7591), and PKCE, using the MCP
// SDK's oauthex helpers plus golang.org/x/oauth2. Secrets (PKCE verifier, client
// secret, tokens) are encrypted at rest with the same key the settings service
// uses; the store holds them as opaque bytes.
//
// The flow is web-native: Initiate returns an authorization URL (the caller
// redirects the admin's browser), and Complete finishes at the OAuth callback.
// At runtime, TokenSource returns an auto-refreshing token source that persists
// refreshed tokens back to the connection row.
package mcpoauth

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/irfanmaulana007/personal-assistant/app/api/internal/authctx"
	"github.com/irfanmaulana007/personal-assistant/app/api/internal/crypto"
	"github.com/irfanmaulana007/personal-assistant/app/api/internal/store"
	"github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/oauthex"
	"golang.org/x/oauth2"
)

const (
	httpTimeout = 30 * time.Second
	pendingTTL  = 15 * time.Minute
	clientName  = "Personal Assistant"

	// initBody is a minimal MCP initialize request used to probe an endpoint for
	// its 401 WWW-Authenticate challenge (which points at the resource metadata).
	initBody = `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"personal-assistant","version":"1.0"}}}`
)

// Store is the slice of the data store this service needs.
type Store interface {
	CreateMCPOAuthPending(ctx context.Context, p store.MCPOAuthPending) error
	GetMCPOAuthPending(ctx context.Context, state string) (*store.MCPOAuthPending, error)
	DeleteMCPOAuthPending(ctx context.Context, state string) error
	UpsertMCPOAuthConnection(ctx context.Context, c store.MCPOAuthConnection) error
	GetMCPOAuthConnection(ctx context.Context, provider string) (*store.MCPOAuthConnection, error)
	UpdateMCPOAuthToken(ctx context.Context, projectID int64, provider string, accessEnc, refreshEnc []byte, expiry *time.Time) error
	DeleteMCPOAuthConnection(ctx context.Context, provider string) error
}

// Service runs the MCP OAuth flow and owns token encryption.
type Service struct {
	store  Store
	encKey []byte
	http   *http.Client
	log    *slog.Logger
}

// New creates an MCP OAuth service. encKey is the AES key (nil ⇒ plaintext, dev).
func New(st Store, encKey []byte, log *slog.Logger) *Service {
	return &Service{
		store:  st,
		encKey: encKey,
		http:   &http.Client{Timeout: httpTimeout},
		log:    log.With("component", "mcp-oauth"),
	}
}

func (s *Service) enc(b []byte) ([]byte, error) { return crypto.Encrypt(s.encKey, b) }
func (s *Service) dec(b []byte) ([]byte, error) { return crypto.Decrypt(s.encKey, b) }

// serverMeta is the discovered authorization-server configuration for a provider.
type serverMeta struct {
	authEndpoint  string
	tokenEndpoint string
	regEndpoint   string
	scopes        []string
}

// Initiate discovers the provider's authorization server, dynamically registers
// a client, and returns the authorization URL to redirect the browser to.
// endpoint is the MCP endpoint (resource); publicOrigin is the app's public
// origin (scheme+host) used to build the OAuth callback URL.
func (s *Service) Initiate(ctx context.Context, provider, endpoint, publicOrigin string) (string, error) {
	meta, err := s.discover(ctx, endpoint)
	if err != nil {
		return "", fmt.Errorf("discover %s oauth: %w", provider, err)
	}
	if meta.regEndpoint == "" {
		return "", fmt.Errorf("%s does not advertise a dynamic client registration endpoint", provider)
	}

	redirectURI := strings.TrimRight(publicOrigin, "/") + "/api/integrations/mcp/" + provider + "/oauth/callback"
	reg, err := oauthex.RegisterClient(ctx, meta.regEndpoint, &oauthex.ClientRegistrationMetadata{
		RedirectURIs:  []string{redirectURI},
		GrantTypes:    []string{"authorization_code", "refresh_token"},
		ResponseTypes: []string{"code"},
		ClientName:    clientName,
	}, s.http)
	if err != nil {
		return "", fmt.Errorf("register %s client: %w", provider, err)
	}

	conf := s.oauthConfig(reg.ClientID, reg.ClientSecret, meta, redirectURI)
	verifier := oauth2.GenerateVerifier()
	state, err := randomState()
	if err != nil {
		return "", err
	}
	authURL := conf.AuthCodeURL(state,
		oauth2.S256ChallengeOption(verifier),
		oauth2.AccessTypeOffline,
		oauth2.SetAuthURLParam("resource", endpoint),
	)

	secretEnc, err := s.enc([]byte(reg.ClientSecret))
	if err != nil {
		return "", err
	}
	verifierEnc, err := s.enc([]byte(verifier))
	if err != nil {
		return "", err
	}
	if err := s.store.CreateMCPOAuthPending(ctx, store.MCPOAuthPending{
		State:           state,
		ProjectID:       authctx.ProjectID(ctx),
		Provider:        provider,
		VerifierEnc:     verifierEnc,
		ClientID:        reg.ClientID,
		ClientSecretEnc: secretEnc,
		AuthEndpoint:    meta.authEndpoint,
		TokenEndpoint:   meta.tokenEndpoint,
		RedirectURI:     redirectURI,
		Resource:        endpoint,
		Scopes:          strings.Join(meta.scopes, " "),
	}); err != nil {
		return "", err
	}
	return authURL, nil
}

// Complete exchanges the authorization code for tokens and stores the connection.
// Returns the project and provider the connection belongs to (from the pending
// row) so the caller can redirect appropriately.
func (s *Service) Complete(ctx context.Context, state, code string) (projectID int64, provider string, err error) {
	p, err := s.store.GetMCPOAuthPending(ctx, state)
	if err != nil {
		return 0, "", err
	}
	if p == nil {
		return 0, "", fmt.Errorf("unknown or expired authorization state")
	}
	// Best-effort cleanup regardless of outcome.
	defer func() { _ = s.store.DeleteMCPOAuthPending(ctx, state) }()

	if time.Since(p.CreatedAt) > pendingTTL {
		return 0, "", fmt.Errorf("authorization expired; please reconnect")
	}

	verifier, err := s.dec(p.VerifierEnc)
	if err != nil {
		return 0, "", err
	}
	secret, err := s.dec(p.ClientSecretEnc)
	if err != nil {
		return 0, "", err
	}

	conf := s.oauthConfig(p.ClientID, string(secret), serverMeta{
		authEndpoint:  p.AuthEndpoint,
		tokenEndpoint: p.TokenEndpoint,
		scopes:        splitScopes(p.Scopes),
	}, p.RedirectURI)

	tok, err := conf.Exchange(ctx, code,
		oauth2.VerifierOption(string(verifier)),
		oauth2.SetAuthURLParam("resource", p.Resource),
	)
	if err != nil {
		return 0, "", fmt.Errorf("token exchange: %w", err)
	}

	accessEnc, err := s.enc([]byte(tok.AccessToken))
	if err != nil {
		return 0, "", err
	}
	refreshEnc, err := s.enc([]byte(tok.RefreshToken))
	if err != nil {
		return 0, "", err
	}
	var expiry *time.Time
	if !tok.Expiry.IsZero() {
		e := tok.Expiry.UTC()
		expiry = &e
	}
	if err := s.store.UpsertMCPOAuthConnection(ctx, store.MCPOAuthConnection{
		ProjectID:       p.ProjectID,
		Provider:        p.Provider,
		ClientID:        p.ClientID,
		ClientSecretEnc: p.ClientSecretEnc,
		TokenEndpoint:   p.TokenEndpoint,
		AccessTokenEnc:  accessEnc,
		RefreshTokenEnc: refreshEnc,
		Expiry:          expiry,
		Scopes:          p.Scopes,
	}); err != nil {
		return 0, "", err
	}
	return p.ProjectID, p.Provider, nil
}

// Status reports whether the active project has a stored connection for provider.
func (s *Service) Status(ctx context.Context, provider string) (bool, error) {
	c, err := s.store.GetMCPOAuthConnection(ctx, provider)
	if err != nil {
		return false, err
	}
	return c != nil, nil
}

// Disconnect removes the active project's connection for provider.
func (s *Service) Disconnect(ctx context.Context, provider string) error {
	return s.store.DeleteMCPOAuthConnection(ctx, provider)
}

// TokenSource returns an auto-refreshing token source for the active project's
// connection, or an error when there is no connection. Refreshed tokens are
// persisted back to the connection row.
func (s *Service) TokenSource(ctx context.Context, provider string) (oauth2.TokenSource, error) {
	c, err := s.store.GetMCPOAuthConnection(ctx, provider)
	if err != nil {
		return nil, err
	}
	if c == nil {
		return nil, fmt.Errorf("%s is not connected for this project", provider)
	}
	secret, err := s.dec(c.ClientSecretEnc)
	if err != nil {
		return nil, err
	}
	access, err := s.dec(c.AccessTokenEnc)
	if err != nil {
		return nil, err
	}
	refresh, err := s.dec(c.RefreshTokenEnc)
	if err != nil {
		return nil, err
	}
	conf := &oauth2.Config{
		ClientID:     c.ClientID,
		ClientSecret: string(secret),
		Endpoint:     oauth2.Endpoint{TokenURL: c.TokenEndpoint, AuthStyle: oauth2.AuthStyleAutoDetect},
	}
	tok := &oauth2.Token{AccessToken: string(access), RefreshToken: string(refresh)}
	if c.Expiry != nil {
		tok.Expiry = *c.Expiry
	}
	return &persistingSource{
		svc:       s,
		projectID: c.ProjectID,
		provider:  provider,
		base:      conf.TokenSource(ctx, tok),
		last:      string(access),
	}, nil
}

// persistingSource wraps an oauth2 token source and writes refreshed tokens back
// to the store when the access token changes.
type persistingSource struct {
	svc       *Service
	projectID int64
	provider  string
	base      oauth2.TokenSource
	last      string
}

func (p *persistingSource) Token() (*oauth2.Token, error) {
	t, err := p.base.Token()
	if err != nil {
		return nil, err
	}
	if t.AccessToken != p.last {
		p.last = t.AccessToken
		accessEnc, e1 := p.svc.enc([]byte(t.AccessToken))
		refreshEnc, e2 := p.svc.enc([]byte(t.RefreshToken))
		if e1 == nil && e2 == nil {
			var expiry *time.Time
			if !t.Expiry.IsZero() {
				e := t.Expiry.UTC()
				expiry = &e
			}
			if err := p.svc.store.UpdateMCPOAuthToken(context.Background(), p.projectID, p.provider, accessEnc, refreshEnc, expiry); err != nil {
				p.svc.log.Warn("persist refreshed mcp token", "provider", p.provider, "error", err)
			}
		}
	}
	return t, nil
}

// oauthConfig builds an oauth2.Config from a provider's server metadata.
func (s *Service) oauthConfig(clientID, clientSecret string, meta serverMeta, redirectURI string) *oauth2.Config {
	return &oauth2.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		Endpoint: oauth2.Endpoint{
			AuthURL:   meta.authEndpoint,
			TokenURL:  meta.tokenEndpoint,
			AuthStyle: oauth2.AuthStyleAutoDetect,
		},
		RedirectURL: redirectURI,
		Scopes:      meta.scopes,
	}
}

// discover finds the provider's authorization server via RFC 9728 → RFC 8414.
func (s *Service) discover(ctx context.Context, endpoint string) (serverMeta, error) {
	prmURL := s.probeResourceMetadataURL(ctx, endpoint)
	if prmURL == "" {
		if u, err := url.Parse(endpoint); err == nil {
			prmURL = u.Scheme + "://" + u.Host + "/.well-known/oauth-protected-resource"
		}
	}
	prm, err := oauthex.GetProtectedResourceMetadata(ctx, prmURL, endpoint, s.http)
	if err != nil {
		return serverMeta{}, fmt.Errorf("protected resource metadata: %w", err)
	}
	if len(prm.AuthorizationServers) == 0 {
		return serverMeta{}, fmt.Errorf("no authorization server advertised for %s", endpoint)
	}
	asm, err := auth.GetAuthServerMetadata(ctx, prm.AuthorizationServers[0], s.http)
	if err != nil {
		return serverMeta{}, fmt.Errorf("authorization server metadata: %w", err)
	}
	scopes := prm.ScopesSupported
	if len(scopes) == 0 {
		scopes = asm.ScopesSupported
	}
	return serverMeta{
		authEndpoint:  asm.AuthorizationEndpoint,
		tokenEndpoint: asm.TokenEndpoint,
		regEndpoint:   asm.RegistrationEndpoint,
		scopes:        scopes,
	}, nil
}

// probeResourceMetadataURL sends an unauthenticated request and reads the
// resource_metadata pointer from the 401 WWW-Authenticate challenge. Returns ""
// when unavailable (the caller falls back to the well-known path).
func (s *Service) probeResourceMetadataURL(ctx context.Context, endpoint string) string {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(initBody))
	if err != nil {
		return ""
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	resp, err := s.http.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	challenges, err := oauthex.ParseWWWAuthenticate(resp.Header.Values("WWW-Authenticate"))
	if err != nil {
		return ""
	}
	for _, c := range challenges {
		if v := c.Params["resource_metadata"]; v != "" {
			return v
		}
	}
	return ""
}

func splitScopes(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	return strings.Fields(s)
}

func randomState() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate state: %w", err)
	}
	return hex.EncodeToString(b), nil
}
