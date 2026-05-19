package config

import (
	"strings"
	"testing"
)

func TestDatabaseConfigDSNPreservesSlashTimezone(t *testing.T) {
	dsn := DatabaseConfig{
		Host:     "192.0.2.10",
		Port:     5432,
		User:     "b_alpha",
		Password: "placeholder",
		Name:     "b_alpha",
		SSLMode:  "disable",
		TimeZone: "Asia/Shanghai",
	}.DSN()

	if !strings.Contains(dsn, "TimeZone=Asia/Shanghai") {
		t.Fatalf("DSN should preserve slash timezone, got %q", dsn)
	}
	if strings.Contains(dsn, "Asia%2FShanghai") {
		t.Fatalf("DSN should not URL-encode timezone, got %q", dsn)
	}
}

func TestDatabaseConfigDSNQuotesSpecialPasswordCharacters(t *testing.T) {
	dsn := DatabaseConfig{
		Host:     "localhost",
		Port:     5432,
		User:     "b_alpha",
		Password: `pa ss'word\`,
		Name:     "b_alpha",
	}.DSN()

	if !strings.Contains(dsn, `password='pa ss\'word\\'`) {
		t.Fatalf("DSN should quote and escape special password characters, got %q", dsn)
	}
}
