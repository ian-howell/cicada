package store

import (
	"testing"
)

func TestNew_AppliesMigrations(t *testing.T) {
	s, err := New(t.TempDir())
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer s.Close()

	// Verify both tables exist by querying them.
	for _, table := range []string{"builds", "step_results"} {
		row := s.db.QueryRow("SELECT count(*) FROM " + table)
		var count int
		if err := row.Scan(&count); err != nil {
			t.Errorf("table %s not found or not queryable: %v", table, err)
		}
	}
}
