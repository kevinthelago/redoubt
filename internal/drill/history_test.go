package drill

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestHistoryRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "history.json")

	h, err := LoadHistory(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(h.Results) != 0 {
		t.Errorf("expected empty history, got %d results", len(h.Results))
	}

	r := Result{
		Timestamp:  time.Now().UTC().Truncate(time.Second),
		Source:     SourceVault,
		SnapshotID: "abc123",
		Restored:   5,
		Verified:   5,
		Status:     StatusPass,
	}
	h.Append(r, 10)
	if err := h.Save(path); err != nil {
		t.Fatal(err)
	}

	h2, err := LoadHistory(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(h2.Results) != 1 {
		t.Fatalf("expected 1 result after reload, got %d", len(h2.Results))
	}
	got := h2.Results[0]
	if got.Source != SourceVault || got.Status != StatusPass || got.Verified != 5 {
		t.Errorf("round-trip mismatch: %+v", got)
	}
}

func TestHistoryMaxLen(t *testing.T) {
	h := &History{}
	for i := 0; i < 5; i++ {
		h.Append(Result{Source: SourceVault, Status: StatusPass}, 3)
	}
	if len(h.Results) != 3 {
		t.Errorf("expected 3 entries after cap, got %d", len(h.Results))
	}
}

func TestHistoryLast(t *testing.T) {
	h := &History{}
	vaultResult := Result{Source: SourceVault, Status: StatusPass, SnapshotID: "v1"}
	coldResult := Result{Source: SourceCold, Status: StatusFail, SnapshotID: "c1"}
	h.Append(coldResult, 100)
	h.Append(vaultResult, 100)

	got, ok := h.Last(SourceVault)
	if !ok || got.SnapshotID != "v1" {
		t.Errorf("Last(vault) = %+v, ok=%v", got, ok)
	}
	got, ok = h.Last(SourceCold)
	if !ok || got.SnapshotID != "c1" {
		t.Errorf("Last(cold) = %+v, ok=%v", got, ok)
	}
	_, ok = h.Last("other")
	if ok {
		t.Error("expected false for unknown source")
	}
}

func TestWriteSummary(t *testing.T) {
	dir := t.TempDir()
	statusPath := filepath.Join(dir, "status.json")
	h := &History{}
	h.Append(Result{Source: SourceVault, Status: StatusPass}, 10)

	if err := h.WriteSummary(statusPath); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(statusPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) == 0 {
		t.Error("summary file is empty")
	}
}
