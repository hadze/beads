package cost

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func tempStore(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	beadsDir := filepath.Join(dir, ".beads")
	s, err := Open(beadsDir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestTrackAndGet(t *testing.T) {
	s := tempStore(t)

	sess := Session{
		ID:           "test-1",
		BeadID:       "PROJ-42",
		Model:        "claude-sonnet-4-5-20250929",
		Provider:     "anthropic",
		InputTokens:  1500,
		OutputTokens: 500,
		CostUSD:      0.0123,
		DurationSec:  4.5,
		Note:         "test session",
		CreatedAt:    time.Date(2025, 1, 15, 10, 0, 0, 0, time.UTC),
	}
	if err := s.Track(sess); err != nil {
		t.Fatalf("Track: %v", err)
	}

	got, err := s.GetSession("test-1")
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if got == nil {
		t.Fatal("GetSession returned nil")
	}
	if got.Model != "claude-sonnet-4-5-20250929" {
		t.Errorf("Model = %q, want claude-sonnet-4-5-20250929", got.Model)
	}
	if got.InputTokens != 1500 {
		t.Errorf("InputTokens = %d, want 1500", got.InputTokens)
	}
	if got.CostUSD != 0.0123 {
		t.Errorf("CostUSD = %f, want 0.0123", got.CostUSD)
	}
	if got.BeadID != "PROJ-42" {
		t.Errorf("BeadID = %q, want PROJ-42", got.BeadID)
	}
}

func TestTrackRequiresID(t *testing.T) {
	s := tempStore(t)
	err := s.Track(Session{})
	if err == nil {
		t.Fatal("expected error for empty ID")
	}
}

func TestListSessions(t *testing.T) {
	s := tempStore(t)

	for i := 0; i < 5; i++ {
		sess := Session{
			ID:          fmt.Sprintf("s-%d", i),
			Model:       "gpt-4",
			CostUSD:     float64(i) * 0.01,
			InputTokens: int64(i) * 100,
			CreatedAt:   time.Date(2025, 1, 1+i, 0, 0, 0, 0, time.UTC),
		}
		if err := s.Track(sess); err != nil {
			t.Fatalf("Track %d: %v", i, err)
		}
	}

	all, err := s.ListSessions(0)
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	if len(all) != 5 {
		t.Fatalf("ListSessions(0) = %d sessions, want 5", len(all))
	}
	if all[0].ID != "s-4" {
		t.Errorf("first session = %q, want s-4", all[0].ID)
	}

	limited, err := s.ListSessions(2)
	if err != nil {
		t.Fatalf("ListSessions(2): %v", err)
	}
	if len(limited) != 2 {
		t.Fatalf("ListSessions(2) = %d sessions, want 2", len(limited))
	}
}

func TestSessionsForBead(t *testing.T) {
	s := tempStore(t)

	s.Track(Session{ID: "a", BeadID: "PROJ-1", CostUSD: 0.01, CreatedAt: time.Now()})
	s.Track(Session{ID: "b", BeadID: "PROJ-2", CostUSD: 0.02, CreatedAt: time.Now()})
	s.Track(Session{ID: "c", BeadID: "PROJ-1", CostUSD: 0.03, CreatedAt: time.Now()})

	got, err := s.SessionsForBead("PROJ-1")
	if err != nil {
		t.Fatalf("SessionsForBead: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("SessionsForBead(PROJ-1) = %d, want 2", len(got))
	}
}

func TestStatus(t *testing.T) {
	s := tempStore(t)

	s.Track(Session{ID: "x", CostUSD: 0.10, InputTokens: 1000, OutputTokens: 500, DurationSec: 10, CreatedAt: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)})
	s.Track(Session{ID: "y", CostUSD: 0.20, InputTokens: 2000, OutputTokens: 800, DurationSec: 20, CreatedAt: time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC)})

	sum, err := s.Status()
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if sum.TotalSessions != 2 {
		t.Errorf("TotalSessions = %d, want 2", sum.TotalSessions)
	}
	if math.Abs(sum.TotalCostUSD-0.30) > 1e-9 {
		t.Errorf("TotalCostUSD = %f, want 0.30", sum.TotalCostUSD)
	}
	if sum.TotalInput != 3000 {
		t.Errorf("TotalInput = %d, want 3000", sum.TotalInput)
	}
	if sum.TotalOutput != 1300 {
		t.Errorf("TotalOutput = %d, want 1300", sum.TotalOutput)
	}
}

func TestStatusEmpty(t *testing.T) {
	s := tempStore(t)
	sum, err := s.Status()
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if sum.TotalSessions != 0 {
		t.Errorf("TotalSessions = %d, want 0", sum.TotalSessions)
	}
}

func TestGetSessionNotFound(t *testing.T) {
	s := tempStore(t)
	got, err := s.GetSession("nonexistent")
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil for nonexistent session, got %+v", got)
	}
}

func TestOpenCreatesDir(t *testing.T) {
	dir := t.TempDir()
	beadsDir := filepath.Join(dir, "deep", "nested", ".beads")
	s, err := Open(beadsDir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()

	if _, err := os.Stat(beadsDir); err != nil {
		t.Errorf("beadsDir not created: %v", err)
	}
}
