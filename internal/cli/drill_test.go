package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/kevinthelago/redoubt/internal/drill"
)

// TestDrillRunCmdHelp verifies the command tree is wired correctly.
func TestDrillRunCmdHelp(t *testing.T) {
	cmd := NewDrillCmd()
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"--help"})
	_ = cmd.Execute()
	if !strings.Contains(buf.String(), "restore drill") && !strings.Contains(buf.String(), "drill") {
		t.Errorf("expected drill help text, got: %q", buf.String())
	}
}

func TestDrillHistoryCmdEmpty(t *testing.T) {
	cmd := NewDrillCmd()
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	// history with an intentionally nonexistent path is handled by LoadHistory
	cmd.SetArgs([]string{"history", "--json"})
	_ = cmd.Execute()
	// Should not panic; output may be "[]" or similar
}

func TestPrintResult(t *testing.T) {
	r := drill.Result{
		Source:   drill.SourceVault,
		Status:   drill.StatusPass,
		Verified: 5,
		Restored: 5,
	}
	// Just confirm it doesn't panic.
	printResult(r)
}

func TestPrintHistory(t *testing.T) {
	results := []drill.Result{
		{Source: drill.SourceVault, Status: drill.StatusPass, Verified: 3},
		{Source: drill.SourceCold, Status: drill.StatusSkip, SkipReason: "no snapshots"},
	}
	printHistory(results)
}

func TestParseSource(t *testing.T) {
	cases := []struct {
		in  string
		ok  bool
		out drill.Source
	}{
		{"vault", true, drill.SourceVault},
		{"cold", true, drill.SourceCold},
		{"unknown", false, ""},
		{"", false, ""},
	}
	for _, c := range cases {
		got, err := parseSource(c.in)
		if (err == nil) != c.ok {
			t.Errorf("parseSource(%q): err=%v, want ok=%v", c.in, err, c.ok)
		}
		if err == nil && got != c.out {
			t.Errorf("parseSource(%q) = %q, want %q", c.in, got, c.out)
		}
	}
}

func TestPrintJSON(t *testing.T) {
	v := map[string]any{"status": "pass", "verified": 3}
	if err := printJSON(v); err != nil {
		t.Errorf("printJSON: %v", err)
	}
}

func TestCadenceStateRoundTrip(t *testing.T) {
	cs := cadenceState{
		VaultCadence: drill.DefaultVaultCadence,
		ColdCadence:  drill.DefaultColdCadence,
	}
	data, err := json.Marshal(cs)
	if err != nil {
		t.Fatal(err)
	}
	var cs2 cadenceState
	if err := json.Unmarshal(data, &cs2); err != nil {
		t.Fatal(err)
	}
	if cs.VaultCadence != cs2.VaultCadence || cs.ColdCadence != cs2.ColdCadence {
		t.Errorf("round-trip mismatch: %+v vs %+v", cs, cs2)
	}
}

func TestStubRestorer(t *testing.T) {
	s := &stubRestorer{}
	ctx := context.Background()

	_, _, err := s.LatestSnapshotID(ctx, drill.SourceVault)
	if err == nil {
		t.Error("expected error from stub LatestSnapshotID")
	}
	_, err = s.ListSnapshotFiles(ctx, drill.SourceVault, "snap", "")
	if err == nil {
		t.Error("expected error from stub ListSnapshotFiles")
	}
	err = s.RestoreFiles(ctx, drill.SourceVault, "snap", nil, "/tmp")
	if err == nil {
		t.Error("expected error from stub RestoreFiles")
	}
	// FreeSpaceAt should work (calls OS)
	_, _ = s.FreeSpaceAt(t.TempDir())
}
