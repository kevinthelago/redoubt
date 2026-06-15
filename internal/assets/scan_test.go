package assets_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kevinthelago/redoubt/internal/assets"
)

func TestScanner_Clean(t *testing.T) {
	dir := writeFile(t, "clean.go", `package main

func main() {
    println("hello world")
}
`)
	a := assets.Asset{Path: dir, Category: assets.CategorySource}
	results, err := assets.DefaultScanner().ScanAssets([]assets.Asset{a}, nil)
	if err != nil {
		t.Fatalf("ScanAssets: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected no findings, got %d: %+v", len(results), results)
	}
}

func TestScanner_AWSKey(t *testing.T) {
	dir := writeFile(t, "config.py", `AWS_ACCESS_KEY_ID = "AKIAIOSFODNN7EXAMPLE"
AWS_SECRET_ACCESS_KEY = "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY"
`)
	a := assets.Asset{Path: dir, Category: assets.CategoryConfig}
	results, err := assets.DefaultScanner().ScanAssets([]assets.Asset{a}, nil)
	if err != nil {
		t.Fatalf("ScanAssets: %v", err)
	}
	if len(results) == 0 {
		t.Error("expected at least one finding for AWS key")
	}
	for _, r := range results {
		if strings.Contains(r.File, "config.py") {
			return // pass
		}
	}
	t.Errorf("expected finding in config.py, got: %+v", results)
}

func TestScanner_PrivateKey(t *testing.T) {
	dir := writeFile(t, "id_rsa", `-----BEGIN RSA PRIVATE KEY-----
MIIEowIBAAKCAQEA0Z3VS5JJcds3xHn/ygWep4ER...
-----END RSA PRIVATE KEY-----
`)
	a := assets.Asset{Path: dir, Category: assets.CategorySource}
	results, err := assets.DefaultScanner().ScanAssets([]assets.Asset{a}, nil)
	if err != nil {
		t.Fatalf("ScanAssets: %v", err)
	}
	if len(results) == 0 {
		t.Error("expected finding for private key")
	}
	for _, r := range results {
		if r.RuleID == "private-key" {
			return // pass
		}
	}
	t.Errorf("expected private-key rule, got: %+v", results)
}

func TestScanner_GitHubToken(t *testing.T) {
	dir := writeFile(t, ".env", `GH_TOKEN=ghp_ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefg123
`)
	a := assets.Asset{Path: dir, Category: assets.CategoryConfig}
	results, err := assets.DefaultScanner().ScanAssets([]assets.Asset{a}, nil)
	if err != nil {
		t.Fatalf("ScanAssets: %v", err)
	}
	found := false
	for _, r := range results {
		if r.RuleID == "github-pat" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected github-pat finding, got: %+v", results)
	}
}

func TestScanner_SkipsSecretsCategory(t *testing.T) {
	dir := writeFile(t, "key.pem", "-----BEGIN RSA PRIVATE KEY-----\ndata\n-----END RSA PRIVATE KEY-----\n")
	// Tagged as secrets — should be skipped.
	a := assets.Asset{Path: dir, Category: assets.CategorySecrets}
	results, err := assets.DefaultScanner().ScanAssets([]assets.Asset{a}, nil)
	if err != nil {
		t.Fatalf("ScanAssets: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("secrets-category asset should not be scanned, got %d results", len(results))
	}
}

func TestScanner_SkipsBinaryFile(t *testing.T) {
	dir := t.TempDir()
	// Write a file with a null byte (binary).
	bin := filepath.Join(dir, "binary.bin")
	data := make([]byte, 100)
	data[10] = 0 // null byte
	copy(data[11:], []byte(`AKIAIOSFODNN7EXAMPLE`))
	if err := os.WriteFile(bin, data, 0o600); err != nil {
		t.Fatal(err)
	}
	a := assets.Asset{Path: dir, Category: assets.CategorySource}
	results, err := assets.DefaultScanner().ScanAssets([]assets.Asset{a}, nil)
	if err != nil {
		t.Fatalf("ScanAssets: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("binary file should be skipped, got %d results", len(results))
	}
}

func TestScanner_ExcluderSkipsFiles(t *testing.T) {
	dir := writeFile(t, "secret.log", `AKIA_TEST_KEY_ID = "AKIAIOSFODNN7EXAMPLE"
`)
	a := assets.Asset{Path: dir, Category: assets.CategorySource}
	ex := assets.NewExcluder([]string{"*.log"})
	results, err := assets.DefaultScanner().ScanAssets([]assets.Asset{a}, ex)
	if err != nil {
		t.Fatalf("ScanAssets: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("excluded file should not produce findings, got %d", len(results))
	}
}

func TestScanner_ResultNeverContainsMatchedValue(t *testing.T) {
	secret := "AKIAIOSFODNN7EXAMPLE"
	dir := writeFile(t, "cfg.env", `AWS_KEY=`+secret+"\n")
	a := assets.Asset{Path: dir, Category: assets.CategoryConfig}
	results, err := assets.DefaultScanner().ScanAssets([]assets.Asset{a}, nil)
	if err != nil {
		t.Fatalf("ScanAssets: %v", err)
	}
	for _, r := range results {
		if strings.Contains(r.File+r.RuleID+r.Description, secret) {
			t.Errorf("result must not contain the matched secret value: %+v", r)
		}
	}
}

// writeFile creates a temp dir, writes name/content into it, and returns the dir path.
func writeFile(t *testing.T, name, content string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
		t.Fatalf("writeFile %s: %v", name, err)
	}
	return dir
}
