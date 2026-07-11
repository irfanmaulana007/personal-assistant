package config

import "testing"

// baseValid returns a config that passes validation, so each test can perturb a
// single field and assert on that field alone.
func baseValid() *Config {
	cfg := defaults()
	cfg.Owner.WhatsAppJID = "123@s.whatsapp.net"
	cfg.Web.Password = "secret"
	cfg.Database.PostgresDSN = "postgres://u:p@localhost:5432/app"
	cfg.Database.MongoURI = "mongodb://localhost:27017"
	// MongoDB defaults to "assistant_logs".
	return cfg
}

func TestValidateDatabase(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*Config)
		wantErr bool
	}{
		{
			name:    "full hybrid config is valid",
			mutate:  func(*Config) {},
			wantErr: false,
		},
		{
			name:    "missing postgres dsn fails",
			mutate:  func(c *Config) { c.Database.PostgresDSN = "" },
			wantErr: true,
		},
		{
			name:    "missing mongo uri fails",
			mutate:  func(c *Config) { c.Database.MongoURI = "" },
			wantErr: true,
		},
		{
			name:    "missing mongo db fails",
			mutate:  func(c *Config) { c.Database.MongoDB = "" },
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := baseValid()
			tt.mutate(cfg)
			err := validate(cfg)
			if tt.wantErr && err == nil {
				t.Fatalf("expected validation error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("expected no error, got %v", err)
			}
		})
	}
}
