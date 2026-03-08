package config

import (
	"path/filepath"
	"testing"
)

func TestStarterConfigPrefillsCoreFields(t *testing.T) {
	cfg := StarterConfig("client-site")

	if cfg.Site.Name != "client-site" {
		t.Fatalf("site name = %q, want %q", cfg.Site.Name, "client-site")
	}
	if cfg.Source.User != "deploy" {
		t.Fatalf("source user = %q, want deploy", cfg.Source.User)
	}
	if cfg.Source.DBName != "client-site_prod" {
		t.Fatalf("source db name = %q", cfg.Source.DBName)
	}
	if cfg.Destination.DBName != "client-site_local" {
		t.Fatalf("destination db name = %q", cfg.Destination.DBName)
	}
	if len(cfg.Replace) != 2 {
		t.Fatalf("replace count = %d, want 2", len(cfg.Replace))
	}
	if len(cfg.Sync) != 1 {
		t.Fatalf("sync count = %d, want 1", len(cfg.Sync))
	}
	if cfg.Sync[0].Src != filepath.Join("/var/www", "client-site") {
		t.Fatalf("sync src = %q", cfg.Sync[0].Src)
	}
	if cfg.Replace[0].Replace != "http://client-site.local" {
		t.Fatalf("replace target = %q", cfg.Replace[0].Replace)
	}
}

func TestValidateConfigName(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		wantErr bool
	}{
		{name: "valid", value: "client-site", wantErr: false},
		{name: "empty", value: "", wantErr: true},
		{name: "dot", value: ".", wantErr: true},
		{name: "slash", value: "team/site", wantErr: true},
		{name: "backslash", value: "team\\site", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateConfigName(tt.value)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ValidateConfigName(%q) error = %v, wantErr %v", tt.value, err, tt.wantErr)
			}
		})
	}
}