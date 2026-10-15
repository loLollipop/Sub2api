package migrations

import (
	"strings"
	"testing"
)

func TestContentModerationEngineMetaMigration(t *testing.T) {
	body, err := FS.ReadFile("265_content_moderation_engine_meta.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := strings.ToUpper(string(body))
	if !strings.Contains(sql, "ADD COLUMN IF NOT EXISTS ENGINE_META JSONB") || strings.Contains(sql, "NOT NULL") || strings.Contains(sql, "DEFAULT") {
		t.Fatalf("engine metadata must be nullable and append-only: %s", sql)
	}
}
