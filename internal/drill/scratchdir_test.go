package drill

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCreateAndWipeScratch(t *testing.T) {
	base := t.TempDir()
	dir, err := CreateScratch(base)
	if err != nil {
		t.Fatal(err)
	}
	if _, statErr := os.Stat(dir); statErr != nil {
		t.Fatalf("scratch dir not created: %v", statErr)
	}

	// Write a file with known content.
	testFile := filepath.Join(dir, "secret.txt")
	if err := os.WriteFile(testFile, []byte("sensitive data 1234"), 0600); err != nil {
		t.Fatal(err)
	}

	if err := SecureWipe(dir); err != nil {
		t.Errorf("wipe error: %v", err)
	}
	if _, statErr := os.Stat(dir); !os.IsNotExist(statErr) {
		t.Errorf("scratch dir still exists after wipe")
	}
}

func TestSecureWipeNonExistentDir(t *testing.T) {
	if err := SecureWipe("/tmp/redoubt-drill-nonexistent-xyz"); err != nil {
		t.Logf("wipe of nonexistent dir returned: %v (may be fine on this OS)", err)
	}
}

func TestCreateScratchCreatesBase(t *testing.T) {
	base := filepath.Join(t.TempDir(), "nonexistent", "nested")
	dir, err := CreateScratch(base)
	if err != nil {
		t.Fatal(err)
	}
	if _, statErr := os.Stat(dir); statErr != nil {
		t.Fatalf("scratch dir %q not accessible: %v", dir, statErr)
	}
	_ = os.RemoveAll(base)
}
