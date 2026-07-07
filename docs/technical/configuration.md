# Configuration

## Config File

Primary configuration via `config.yaml` in the project root.

```yaml
# config.yaml

# Owner identification
owner:
  whatsapp_jid: "6281234567890@s.whatsapp.net"
  name: "Irfan"
  timezone: "Asia/Jakarta"

# Transport settings
whatsapp:
  enabled: true
  database: "data/whatsmeow.db"

# Database
database:
  path: "data/assistant.db"

# Google API credentials
google:
  credentials_file: "config/google_credentials.json"
  # Or via environment variables:
  # client_id: ${GOOGLE_CLIENT_ID}
  # client_secret: ${GOOGLE_CLIENT_SECRET}

# Capabilities
capabilities:
  calendar:
    enabled: true
    default_duration: "1h"    # default event duration
    max_results: 10
  email:
    enabled: true
    max_results: 10
    auto_send: false          # NEVER set to true
  reminders:
    enabled: true
    check_interval: "30s"     # scheduler poll interval
  knowledge:
    enabled: true
    max_note_length: 10000

# Security
security:
  encryption_key: ${ASSISTANT_ENCRYPTION_KEY}

# Logging
logging:
  level: "info"               # debug, info, warn, error
  format: "text"              # text or json
```

## Environment Variable Overrides

Any config value can be overridden with environment variables using the `ASSISTANT_` prefix:

| Config Path | Environment Variable |
|------------|---------------------|
| `owner.whatsapp_jid` | `ASSISTANT_OWNER_WHATSAPP_JID` |
| `database.path` | `ASSISTANT_DATABASE_PATH` |
| `logging.level` | `ASSISTANT_LOGGING_LEVEL` |
| `security.encryption_key` | `ASSISTANT_ENCRYPTION_KEY` |

Environment variables take precedence over config file values.

## Loading Order

1. Load defaults (hardcoded in Go)
2. Load `config.yaml` (overrides defaults)
3. Load environment variables (overrides config file)
4. Validate required fields

```go
func Load() (*Config, error) {
    cfg := defaultConfig()

    // Load YAML
    data, err := os.ReadFile("config.yaml")
    if err == nil {
        yaml.Unmarshal(data, cfg)
    }

    // Load env overrides
    loadEnvOverrides(cfg)

    // Validate
    if err := cfg.Validate(); err != nil {
        return nil, fmt.Errorf("invalid config: %w", err)
    }

    return cfg, nil
}
```

## Validation

Required fields:
- `owner.whatsapp_jid` — must be a valid JID format
- `security.encryption_key` — must be 32 bytes (for AES-256)
- At least one transport enabled
- At least one capability enabled

## Gitignore

```gitignore
# Config with secrets
config.yaml

# Google credentials
config/google_credentials.json

# Runtime data
data/

# Keep example config
!config.example.yaml
```

## Example Config

Ship a `config.example.yaml` with placeholder values for documentation:

```yaml
# config.example.yaml — Copy to config.yaml and fill in your values

owner:
  whatsapp_jid: "62XXXXXXXXXXX@s.whatsapp.net"
  name: "Your Name"
  timezone: "Asia/Jakarta"

whatsapp:
  enabled: true
  database: "data/whatsmeow.db"

database:
  path: "data/assistant.db"

google:
  credentials_file: "config/google_credentials.json"

security:
  encryption_key: ""  # Set via ASSISTANT_ENCRYPTION_KEY env var
```
