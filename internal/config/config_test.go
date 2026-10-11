package config

import (
	"slices"
	"testing"
)

func TestAllowedOrigins(t *testing.T) {
	tests := []struct {
		name, allowed, frontend string
		want                    []string
	}{
		{"nothing set allows any origin", "", "", []string{"*"}},
		{"falls back to frontend URL", "", "https://sort.example/", []string{"https://sort.example"}},
		{"explicit list wins", "https://a.example, https://b.example/", "https://sort.example", []string{"https://a.example", "https://b.example"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("MONGO_URI", "mongodb://localhost")
			t.Setenv("ALLOWED_ORIGINS", tt.allowed)
			t.Setenv("FRONTEND_URL", tt.frontend)
			cfg, err := Load()
			if err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(cfg.AllowedOrigins, tt.want) {
				t.Fatalf("AllowedOrigins = %q, want %q", cfg.AllowedOrigins, tt.want)
			}
		})
	}
}
