package config

import (
	"strings"
	"testing"
	"time"
)

func TestLoad(t *testing.T) {
	for _, tt := range []struct{ name, url, timeout, want string }{
		{"missing", "", "", "DATABASE_URL is required"},
		{"blank", "  ", "", "DATABASE_URL is required"},
		{"invalid timeout", "postgres://localhost/test", "bad", "DATABASE_TIMEOUT"},
		{"zero timeout", "postgres://localhost/test", "0s", "DATABASE_TIMEOUT"},
		{"negative timeout", "postgres://localhost/test", "-1s", "DATABASE_TIMEOUT"},
		{"excessive timeout", "postgres://localhost/test", "61s", "DATABASE_TIMEOUT"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("DATABASE_URL", tt.url)
			t.Setenv("DATABASE_TIMEOUT", tt.timeout)
			_, err := Load()
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want %s", err, tt.want)
			}
		})
	}
	t.Setenv("DATABASE_URL", "postgres://localhost/test")
	t.Setenv("DATABASE_TIMEOUT", "")
	cfg, err := Load()
	if err != nil || cfg.Timeout != 10*time.Second {
		t.Fatalf("config = %+v, error = %v", cfg, err)
	}
	t.Setenv("DATABASE_TIMEOUT", "250ms")
	cfg, err = Load()
	if err != nil || cfg.Timeout != 250*time.Millisecond {
		t.Fatalf("config = %+v, error = %v", cfg, err)
	}
}
