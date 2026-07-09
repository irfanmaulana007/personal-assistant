# Authentication & Security

## Owner Verification

The assistant is single-user. The primary security boundary is verifying that messages come from the owner's WhatsApp JID.

### Configuration

```yaml
owner:
  whatsapp_jid: "6281234567890@s.whatsapp.net"
  name: "Irfan"
```

### Verification Check

```go
func isOwner(msg *transport.Message, config *config.Config) bool {
    return msg.From == config.Owner.WhatsAppJID
}
```

This check happens at the top of the message pipeline. Non-owner messages are silently dropped (no response, no logging of content).

## Google OAuth2

### Setup

1. Create OAuth2 credentials in Google Cloud Console
2. Download credentials JSON
3. Place at `config/google_credentials.json` (gitignored)

### Credential File

```json
{
  "installed": {
    "client_id": "xxx.apps.googleusercontent.com",
    "client_secret": "xxx",
    "redirect_uris": ["http://localhost:8090/callback"]
  }
}
```

### Authorization Flow

```go
func authorize(config *oauth2.Config, store *store.Store) (*http.Client, error) {
    // Check for existing token
    tokenData, err := store.GetOAuthToken("google")
    if err == nil {
        token := decrypt(tokenData)
        return config.Client(context.Background(), token), nil
    }

    // No token — start OAuth flow
    authURL := config.AuthCodeURL("state", oauth2.AccessTypeOffline)
    fmt.Printf("Visit this URL to authorize:\n%s\n", authURL)

    // Start local HTTP server to receive callback
    code := waitForAuthCode()

    token, err := config.Exchange(context.Background(), code)
    store.SaveOAuthToken("google", encrypt(token))

    return config.Client(context.Background(), token), nil
}
```

### Token Storage

Tokens are encrypted before storing in SQLite:

```go
func encrypt(token *oauth2.Token) []byte {
    data, _ := json.Marshal(token)
    // AES-256-GCM encryption using key derived from config
    block, _ := aes.NewCipher(key)
    gcm, _ := cipher.NewGCM(block)
    nonce := make([]byte, gcm.NonceSize())
    io.ReadFull(rand.Reader, nonce)
    return gcm.Seal(nonce, nonce, data, nil)
}
```

Encryption key is derived from a secret in the config file or environment variable.

### Token Refresh

The `oauth2` package handles token refresh automatically. The refreshed token is saved back to the store.

### Required Scopes

| Service | Scopes |
|---------|--------|
| Google Calendar | `calendar.readonly`, `calendar.events` |
| Gmail | `gmail.readonly`, `gmail.compose`, `gmail.labels` |

## Security Best Practices

1. **Never commit credentials** — `config/google_credentials.json` and `config.yaml` are gitignored
2. **Encrypt tokens at rest** — AES-256-GCM encryption in SQLite
3. **Owner-only access** — JID verification on every message
4. **No forwarding** — never forward message content to unauthorized services
5. **Draft-only email** — Gmail integration cannot send, only draft
6. **Audit logging** — all actions logged to `message_log` table
7. **Principle of least privilege** — request minimal OAuth scopes

## Environment Variables

Sensitive values can be provided via environment variables:

```bash
export ASSISTANT_ENCRYPTION_KEY="your-32-byte-key-here"
export GOOGLE_CLIENT_ID="xxx.apps.googleusercontent.com"
export GOOGLE_CLIENT_SECRET="xxx"
```

Environment variables take precedence over config file values.
