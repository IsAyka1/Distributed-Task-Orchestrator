package postgres

import (
	"context"
	"crypto/rand"
	"net/url"
	"strings"
	"testing"
)

func TestOpenRejectsMissingOrInvalidConfiguration(t *testing.T) {
	for _, value := range []string{"", " ", "postgres://%zz"} {
		db, err := Open(context.Background(), value)
		if db != nil || err == nil {
			t.Fatal("invalid configuration was accepted")
		}
	}
}

func TestOpenRedactsDriverFailures(t *testing.T) {
	token := rand.Text()
	for _, suffix := range []string{"", "%zz"} {
		dsn := "postgres://" + url.UserPassword("test", token).String() + suffix + "@127.0.0.1/db"
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		db, err := Open(ctx, dsn)
		if db != nil || err == nil {
			t.Fatal("expected connection failure")
		}
		if strings.Contains(err.Error(), token) {
			t.Fatal("connection error disclosed input")
		}
	}
}
