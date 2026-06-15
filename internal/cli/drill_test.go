package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/kevinthelago/redoubt/internal/drill"
)

// TestDrillCmdHelp verifies the drill subcommand is wired and shows help.
// Cobra's Execute() always traverses to root, so we redirect rootCmd's output.
func TestDrillCmdHelp(t *testing.T) {
	buf := &bytes.Buffer{}
	rootCmd.SetOut(buf)
	rootCmd.SetErr(buf)
	rootCmd.SetArgs([]string{"drill", "--help"})
	_ = rootCmd.Execute()
	out := buf.String()
	if !strings.Contains(out, "drill") {
		t.Errorf("expected 'drill' in help output, got: %q", out)
	}
	rootCmd.SetArgs(nil)
	rootCmd.SetOut(nil)
	rootCmd.SetErr(nil)
}

// TestDrillHistoryCmdHelp confirms the history subcommand is registered.
func TestDrillHistoryCmdHelp(t *testing.T) {
	buf := &bytes.Buffer{}
	rootCmd.SetOut(buf)
	rootCmd.SetErr(buf)
	rootCmd.SetArgs([]string{"drill", "history", "--help"})
	_ = rootCmd.Execute()
	if !strings.Contains(buf.String(), "history") {
		t.Errorf("history subcommand not registered properly")
	}
	rootCmd.SetArgs(nil)
	rootCmd.SetOut(nil)
	rootCmd.SetErr(nil)
}

func TestDrillPrintResult(t *testing.T) {
	r := drill.Result{
		Source:   drill.SourceVault,
		Status:   drill.StatusPass,
		Verified: 5,
		Restored: 5,
	}
	buf := &bytes.Buffer{}
	drillCmd.SetOut(buf)
	drillPrintResult(drillCmd, r)
	if !strings.Contains(buf.String(), "pass") {
		t.Errorf("expected 'pass' in output, got: %q", buf.String())
	}
	drillCmd.SetOut(nil)
}

func TestDrillPrintHistory(t *testing.T) {
	results := []drill.Result{
		{Source: drill.SourceVault, Status: drill.StatusPass, Verified: 3},
		{Source: drill.SourceCold, Status: drill.StatusSkip, SkipReason: "no snapshots"},
	}
	buf := &bytes.Buffer{}
	drillCmd.SetOut(buf)
	drillPrintHistory(drillCmd, results)
	out := buf.String()
	if !strings.Contains(out, "vault") || !strings.Contains(out, "cold") {
		t.Errorf("expected vault and cold in history output, got: %q", out)
	}
	drillCmd.SetOut(nil)
}

func TestParseDrillSource(t *testing.T) {
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
		got, err := parseDrillSource(c.in)
		if (err == nil) != c.ok {
			t.Errorf("parseDrillSource(%q): err=%v, want ok=%v", c.in, err, c.ok)
		}
		if err == nil && got != c.out {
			t.Errorf("parseDrillSource(%q) = %q, want %q", c.in, got, c.out)
		}
	}
}

func TestDrillPrintJSON(t *testing.T) {
	v := map[string]any{"status": "pass", "verified": 3}
	buf := &bytes.Buffer{}
	drillCmd.SetOut(buf)
	if err := drillPrintJSON(drillCmd, v); err != nil {
		t.Errorf("drillPrintJSON: %v", err)
	}
	var roundtrip map[string]any
	if err := json.Unmarshal(buf.Bytes(), &roundtrip); err != nil {
		t.Errorf("output is not valid JSON: %v — output: %q", err, buf.String())
	}
	drillCmd.SetOut(nil)
}

func TestDrillCadenceStateRoundTrip(t *testing.T) {
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
		t.Errorf("cadence state round-trip mismatch: %+v vs %+v", cs, cs2)
	}
}

func TestDrillStubRestorer(t *testing.T) {
	s := &drillStubRestorer{}
	ctx := context.Background()

	_, _, err := s.LatestSnapshotID(ctx, drill.SourceVault)
	if err == nil {
		t.Error("expected error from stub LatestSnapshotID")
	}
	_, err = s.ListSnapshotFiles(ctx, drill.SourceVault, "snap", "")
	if err == nil {
		t.Error("expected error from stub ListSnapshotFiles")
	}
	err = s.RestoreFiles(ctx, drill.SourceVault, "snap", nil, t.TempDir())
	if err == nil {
		t.Error("expected error from stub RestoreFiles")
	}
	// FreeSpaceAt should call the OS — just verify it doesn't panic.
	_, _ = s.FreeSpaceAt(t.TempDir())
}
