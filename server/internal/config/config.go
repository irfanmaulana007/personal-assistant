package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	// Environment names the deployment this instance is running as — typically
	// "local" or "production". It is surfaced in the Logs run-detail copy so a
	// pasted trace makes clear which database (local vs prod) holds its data.
	// Override with the APP_ENV env var; defaults to "local".
	Environment  string             `yaml:"environment"`
	Owner        OwnerConfig        `yaml:"owner"`
	WhatsApp     WhatsAppConfig     `yaml:"whatsapp"`
	Web          WebConfig          `yaml:"web"`
	Database     DatabaseConfig     `yaml:"database"`
	Google       GoogleConfig       `yaml:"google"`
	Capabilities CapabilitiesConfig `yaml:"capabilities"`
	Security     SecurityConfig     `yaml:"security"`
	SMTP         SMTPConfig         `yaml:"smtp"`
	Logging      LoggingConfig      `yaml:"logging"`
}

// LLM provider settings (API key, model, base URL, provider) are NOT configured
// here — they are managed at runtime via the Settings page and stored in the
// database, which is their single source of truth.

type OwnerConfig struct {
	WhatsAppJID string `yaml:"whatsapp_jid"`
	Name        string `yaml:"name"`
	Timezone    string `yaml:"timezone"`
}

func (o OwnerConfig) Location() *time.Location {
	loc, err := time.LoadLocation(o.Timezone)
	if err != nil {
		return time.UTC
	}
	return loc
}

// AllowedJIDs are the WhatsApp numbers permitted to talk to the assistant.
// WhatsAppJID may be a comma-separated list (e.g. your personal + work numbers);
// the assistant replies to whichever of them messages it.
func (o OwnerConfig) AllowedJIDs() []string {
	var out []string
	for _, part := range strings.Split(o.WhatsAppJID, ",") {
		if p := strings.TrimSpace(part); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// PrimaryJID is the first allowed number — where reminders and the daily recap
// are delivered. Empty when none is configured.
func (o OwnerConfig) PrimaryJID() string {
	if j := o.AllowedJIDs(); len(j) > 0 {
		return j[0]
	}
	return ""
}

type WhatsAppConfig struct {
	Enabled bool `yaml:"enabled"`
}

type WebConfig struct {
	Enabled   bool   `yaml:"enabled"`
	Port      int    `yaml:"port"`
	Password  string `yaml:"password"`
	StaticDir string `yaml:"static_dir"`
}

// DatabaseConfig configures the storage backend. The application runs on a
// hybrid backend only — PostgreSQL for main data, MongoDB for logs.
type DatabaseConfig struct {
	// PostgresDSN is the PostgreSQL connection string (main data). Required.
	PostgresDSN string `yaml:"postgres_dsn"`
	// MongoURI is the MongoDB connection string (logs/analytics). Required.
	MongoURI string `yaml:"mongo_uri"`
	// MongoDB is the MongoDB database name for logs. Required.
	MongoDB string `yaml:"mongo_db"`
}

type GoogleConfig struct {
	CredentialsFile string `yaml:"credentials_file"`
}

type CapabilitiesConfig struct {
	Calendar  CalendarConfig  `yaml:"calendar"`
	Email     EmailConfig     `yaml:"email"`
	Reminders ReminderConfig  `yaml:"reminders"`
	Knowledge KnowledgeConfig `yaml:"knowledge"`
}

type CalendarConfig struct {
	Enabled         bool   `yaml:"enabled"`
	DefaultDuration string `yaml:"default_duration"`
	MaxResults      int    `yaml:"max_results"`
}

type EmailConfig struct {
	Enabled  bool `yaml:"enabled"`
	AutoSend bool `yaml:"auto_send"`
}

type ReminderConfig struct {
	Enabled       bool   `yaml:"enabled"`
	CheckInterval string `yaml:"check_interval"`
}

func (r ReminderConfig) CheckIntervalDuration() time.Duration {
	d, err := time.ParseDuration(r.CheckInterval)
	if err != nil {
		return 30 * time.Second
	}
	return d
}

type KnowledgeConfig struct {
	Enabled       bool `yaml:"enabled"`
	MaxNoteLength int  `yaml:"max_note_length"`
}

type SecurityConfig struct {
	EncryptionKey string `yaml:"encryption_key"`
}

// SMTPConfig configures the transactional-email transport used for
// password-reset messages. When Host is empty the feature is disabled and the
// forgot-password endpoint reports it as unavailable. Credentials are supplied
// via the SMTP_* env vars (see applyEnvOverrides) rather than committed YAML.
type SMTPConfig struct {
	Host     string `yaml:"host"`
	Port     int    `yaml:"port"`
	Username string `yaml:"username"`
	Password string `yaml:"password"`
	From     string `yaml:"from"`
	FromName string `yaml:"from_name"`
}

type LoggingConfig struct {
	Level  string `yaml:"level"`
	Format string `yaml:"format"`
}

func Load(path string) (*Config, error) {
	cfg := defaults()

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config file: %w", err)
	}

	// Expand environment variables in the YAML
	expanded := os.ExpandEnv(string(data))

	if err := yaml.Unmarshal([]byte(expanded), cfg); err != nil {
		return nil, fmt.Errorf("parse config file: %w", err)
	}

	applyEnvOverrides(cfg)

	if strings.TrimSpace(cfg.Environment) == "" {
		cfg.Environment = "local"
	}

	if err := validate(cfg); err != nil {
		return nil, fmt.Errorf("validate config: %w", err)
	}

	return cfg, nil
}

func defaults() *Config {
	return &Config{
		Environment: "local",
		Owner: OwnerConfig{
			Timezone: "UTC",
		},
		WhatsApp: WhatsAppConfig{
			Enabled: true,
		},
		Web: WebConfig{
			Enabled:   true,
			Port:      8090,
			StaticDir: "client/dist",
		},
		Database: DatabaseConfig{
			MongoDB: "assistant_logs",
		},
		Google: GoogleConfig{
			CredentialsFile: "config/google_credentials.json",
		},
		Capabilities: CapabilitiesConfig{
			Calendar: CalendarConfig{
				Enabled:         true,
				DefaultDuration: "1h",
				MaxResults:      10,
			},
			Email: EmailConfig{
				Enabled:  true,
				AutoSend: false,
			},
			Reminders: ReminderConfig{
				Enabled:       true,
				CheckInterval: "30s",
			},
			Knowledge: KnowledgeConfig{
				Enabled:       true,
				MaxNoteLength: 10000,
			},
		},
		SMTP: SMTPConfig{
			Port:     587,
			FromName: "Personal Assistant",
		},
		Logging: LoggingConfig{
			Level:  "info",
			Format: "text",
		},
	}
}

func applyEnvOverrides(cfg *Config) {
	if v := os.Getenv("APP_ENV"); v != "" {
		cfg.Environment = v
	}
	if v := os.Getenv("ENCRYPTION_KEY"); v != "" {
		cfg.Security.EncryptionKey = v
	}
	if v := os.Getenv("OWNER_JID"); v != "" {
		cfg.Owner.WhatsAppJID = v
	}
	if v := os.Getenv("POSTGRES_DSN"); v != "" {
		cfg.Database.PostgresDSN = v
	}
	if v := os.Getenv("MONGO_URI"); v != "" {
		cfg.Database.MongoURI = v
	}
	if v := os.Getenv("MONGO_DB"); v != "" {
		cfg.Database.MongoDB = v
	}
	if v := os.Getenv("LOG_LEVEL"); v != "" {
		cfg.Logging.Level = v
	}
	if v := os.Getenv("WEB_PASSWORD"); v != "" {
		cfg.Web.Password = v
	}
	if v := os.Getenv("SMTP_HOST"); v != "" {
		cfg.SMTP.Host = v
	}
	if v := os.Getenv("SMTP_PORT"); v != "" {
		if p, err := strconv.Atoi(v); err == nil {
			cfg.SMTP.Port = p
		}
	}
	if v := os.Getenv("SMTP_USER"); v != "" {
		cfg.SMTP.Username = v
	}
	if v := os.Getenv("SMTP_PASSWORD"); v != "" {
		cfg.SMTP.Password = v
	}
	if v := os.Getenv("SMTP_FROM_ADDRESS"); v != "" {
		cfg.SMTP.From = v
	}
	if v := os.Getenv("SMTP_FROM_NAME"); v != "" {
		cfg.SMTP.FromName = v
	}
}

func validate(cfg *Config) error {
	var errs []string

	if cfg.Owner.WhatsAppJID == "" {
		errs = append(errs, "owner.whatsapp_jid is required")
	}

	// WhatsApp (whatsmeow) persists its session in Postgres too, so it needs no
	// separate database setting — the postgres_dsn check below covers it.

	// The application runs on the hybrid Postgres + Mongo backend only.
	if cfg.Database.PostgresDSN == "" {
		errs = append(errs, "database.postgres_dsn is required (set POSTGRES_DSN env var)")
	}
	if cfg.Database.MongoURI == "" {
		errs = append(errs, "database.mongo_uri is required (set MONGO_URI env var)")
	}
	if cfg.Database.MongoDB == "" {
		errs = append(errs, "database.mongo_db is required")
	}

	if cfg.Web.Enabled && cfg.Web.Password == "" {
		errs = append(errs, "web.password is required when web is enabled (set WEB_PASSWORD env var)")
	}

	// Email auto-send must never be true
	if cfg.Capabilities.Email.AutoSend {
		cfg.Capabilities.Email.AutoSend = false
	}

	if len(errs) > 0 {
		return fmt.Errorf("config validation failed:\n  - %s", strings.Join(errs, "\n  - "))
	}

	return nil
}
