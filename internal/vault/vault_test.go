package vault

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// TestAppendOnlyInvariant verifies that the server args always include --append-only.
// This is the "append-only holds" gate from testing.md.
func TestAppendOnlyInvariant(t *testing.T) {
	cfg := ServiceConfig{
		ListenAddr:    "192.168.1.1:8000",
		RepoDir:       "/tmp/test-repo",
		TLSCertPath:   "/tmp/test-vault/tls/server.crt",
		TLSKeyPath:    "/tmp/test-vault/tls/server.key",
		RestServerBin: "rest-server",
	}

	args := buildServerArgs(cfg)

	found := false
	for _, arg := range args {
		if arg == "--append-only" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("buildServerArgs() does not include --append-only — append-only invariant violated\nargs: %v", args)
	}
}

// TestAppendOnlyCannotBeDropped verifies that no other config variation removes --append-only.
func TestAppendOnlyCannotBeDropped(t *testing.T) {
	// Vary all fields to ensure --append-only is structural, not conditional.
	variants := []ServiceConfig{
		{ListenAddr: "10.0.0.1:443", RepoDir: "/a", TLSCertPath: "/a.crt", TLSKeyPath: "/a.key"},
		{ListenAddr: "172.16.0.1:8080", RepoDir: "/b", TLSCertPath: "/b.crt", TLSKeyPath: "/b.key"},
		{ListenAddr: "192.168.100.100:9000", RepoDir: "/c", TLSCertPath: "/c.crt", TLSKeyPath: "/c.key"},
	}
	for _, cfg := range variants {
		args := buildServerArgs(cfg)
		found := false
		for _, arg := range args {
			if arg == "--append-only" {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("buildServerArgs(%+v) missing --append-only", cfg)
		}
	}
}

func TestFilterEnv(t *testing.T) {
	env := []string{
		"PATH=/usr/bin",
		"RESTIC_PASSWORD=secret",
		"HOME=/root",
		"RESTIC_PASSWORD=another",
		"GOPATH=/go",
	}
	filtered := filterEnv(env, "RESTIC_PASSWORD")
	for _, e := range filtered {
		if e == "RESTIC_PASSWORD=secret" || e == "RESTIC_PASSWORD=another" {
			t.Errorf("filterEnv left RESTIC_PASSWORD in result: %v", filtered)
		}
	}
	if len(filtered) != 3 {
		t.Errorf("filterEnv returned %d entries, want 3: %v", len(filtered), filtered)
	}
}

func TestProfileRepoURL(t *testing.T) {
	p := Profile{Address: "192.168.1.10:8000"}
	want := "rest:https://192.168.1.10:8000/"
	if got := p.RepoURL(); got != want {
		t.Errorf("Profile.RepoURL() = %q, want %q", got, want)
	}
}

func TestWriteReadProfile(t *testing.T) {
	dir := t.TempDir()
	p := Profile{
		Address:       "10.0.0.5:8000",
		CAFingerprint: "abc123def456",
		RepoPath:      "/srv/redoubt/repo",
	}
	path := filepath.Join(dir, "profile.json")
	if err := WriteProfile(path, p); err != nil {
		t.Fatalf("WriteProfile: %v", err)
	}

	got, err := ReadProfile(path)
	if err != nil {
		t.Fatalf("ReadProfile: %v", err)
	}
	if *got != p {
		t.Errorf("round-trip mismatch: got %+v, want %+v", *got, p)
	}
}

func TestReadProfileIncomplete(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "profile.json")
	// Write a profile missing ca_fingerprint.
	if err := os.WriteFile(path, []byte(`{"address":"10.0.0.1:8000"}`), 0600); err != nil {
		t.Fatal(err)
	}
	_, err := ReadProfile(path)
	if err == nil {
		t.Fatal("ReadProfile accepted incomplete profile, want error")
	}
}

func TestInitRejectsPublicAddress(t *testing.T) {
	publicAddrs := []string{
		"0.0.0.0:8000",
		"8.8.8.8:8000",
		"127.0.0.1:8000",
	}
	for _, addr := range publicAddrs {
		_, err := InitWithBase(addr, "/tmp/repo", t.TempDir(), "password")
		if err == nil {
			t.Errorf("InitWithBase(%q) = nil, want error", addr)
			continue
		}
		if !errors.Is(err, ErrPublicAddress) {
			t.Errorf("InitWithBase(%q) = %v, want errors.Is(ErrPublicAddress)", addr, err)
		}
	}
}

func TestInitIdempotent(t *testing.T) {
	dir := t.TempDir()
	// Pre-populate a valid profile.
	p := Profile{
		Address:       "192.168.1.1:8000",
		CAFingerprint: "deadbeef",
		RepoPath:      filepath.Join(dir, "repo"),
	}
	if err := WriteProfile(filepath.Join(dir, "profile.json"), p); err != nil {
		t.Fatal(err)
	}
	// Calling Init on a vault that already has a profile must return ErrAlreadyInited.
	repoPath := filepath.Join(dir, "repo")
	got, err := InitWithBase("192.168.1.1:8000", repoPath, dir, "password")
	if !errors.Is(err, ErrAlreadyInited) {
		t.Fatalf("InitWithBase on existing vault: err = %v, want ErrAlreadyInited", err)
	}
	if got == nil {
		t.Fatal("InitWithBase on existing vault returned nil profile")
	}
	if got.CAFingerprint != p.CAFingerprint {
		t.Errorf("existing profile fingerprint changed: got %q, want %q", got.CAFingerprint, p.CAFingerprint)
	}
}

// TestGenerateCACert verifies that CA generation produces a non-empty fingerprint.
func TestGenerateCACert(t *testing.T) {
	ca, err := GenerateCACert()
	if err != nil {
		t.Fatalf("GenerateCACert: %v", err)
	}
	fp := ca.Fingerprint()
	if len(fp) == 0 {
		t.Fatal("CA fingerprint is empty")
	}
	// SHA-256 hex = 64 characters.
	if len(fp) != 64 {
		t.Errorf("CA fingerprint length = %d, want 64 (SHA-256 hex)", len(fp))
	}
	certPEM := ca.CertPEM()
	if len(certPEM) == 0 {
		t.Fatal("CA cert PEM is empty")
	}
}

func TestGenerateServerCert(t *testing.T) {
	ca, err := GenerateCACert()
	if err != nil {
		t.Fatalf("GenerateCACert: %v", err)
	}
	sc, err := ca.GenerateServerCert("192.168.1.1")
	if err != nil {
		t.Fatalf("GenerateServerCert: %v", err)
	}
	if len(sc.certPEM) == 0 || len(sc.keyPEM) == 0 {
		t.Fatal("server cert/key PEM is empty")
	}
}

func TestWriteTLSFiles(t *testing.T) {
	dir := t.TempDir()
	ca, err := GenerateCACert()
	if err != nil {
		t.Fatal(err)
	}
	if err := ca.WriteTo(dir); err != nil {
		t.Fatalf("CA.WriteTo: %v", err)
	}
	for _, f := range []string{"ca.crt", "ca.key"} {
		if _, err := os.Stat(filepath.Join(dir, f)); err != nil {
			t.Errorf("missing %s after WriteTo", f)
		}
	}

	sc, err := ca.GenerateServerCert("10.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	if err := sc.WriteTo(dir); err != nil {
		t.Fatalf("ServerCert.WriteTo: %v", err)
	}
	for _, f := range []string{"server.crt", "server.key"} {
		if _, err := os.Stat(filepath.Join(dir, f)); err != nil {
			t.Errorf("missing %s after WriteTo", f)
		}
	}
}
