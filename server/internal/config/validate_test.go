package config

import "testing"

// baseValid returns a config that passes validation, so each test can perturb a
// single field and assert on that field alone.
func baseValid() *Config {
	cfg := defaults()
	cfg.Owner.WhatsAppJID = "123@s.whatsapp.net"
	cfg.Web.Password = "secret"
	return cfg
}

func TestValidateDatabaseDriver(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*Config)
		wantErr bool
	}{
		{
			name:    "sqlite default is valid",
			mutate:  func(*Config) {},
			wantErr: false,
		},
		{
			name:    "sqlite requires path",
			mutate:  func(c *Config) { c.Database.Path = "" },
			wantErr: true,
		},
		{
			name: "hybrid requires postgres, mongo uri and db",
			mutate: func(c *Config) {
				c.Database.Driver = DriverHybrid
				c.Database.PostgresDSN = "postgres://u:p@localhost:5432/app"
				c.Database.MongoURI = "mongodb://localhost:27017"
				c.Database.MongoDB = "assistant_logs"
			},
			wantErr: false,
		},
		{
			name: "hybrid missing postgres dsn fails",
			mutate: func(c *Config) {
				c.Database.Driver = DriverHybrid
				c.Database.MongoURI = "mongodb://localhost:27017"
				c.Database.MongoDB = "assistant_logs"
			},
			wantErr: true,
		},
		{
			name: "hybrid missing mongo uri fails",
			mutate: func(c *Config) {
				c.Database.Driver = DriverHybrid
				c.Database.PostgresDSN = "postgres://u:p@localhost:5432/app"
				c.Database.MongoDB = "assistant_logs"
			},
			wantErr: true,
		},
		{
			name: "unknown driver is rejected",
			mutate: func(c *Config) {
				c.Database.Driver = "cassandra"
			},
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
