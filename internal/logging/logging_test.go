package logging_test

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"

	"github.com/kevinthelago/redoubt/internal/logging"
)

const (
	seededSecret  = "s3cr3t-p4ssw0rd-xyz"
	seededSecret2 = "tok3n-abc123-DEF456"
)

// capture builds a logger that writes to a buffer and returns both.
func capture(t *testing.T, secrets ...string) (*bytes.Buffer, *slog.Logger) {
	t.Helper()
	var buf bytes.Buffer
	return &buf, logging.New(&buf, slog.LevelDebug, secrets...)
}

// TestSecretNeverAppears is the core contract: seeded secrets must not appear
// anywhere in captured log output.
func TestSecretNeverAppears(t *testing.T) {
	buf, log := capture(t, seededSecret, seededSecret2)

	log.Info("connecting to vault", "url", "rest:https://vault.lan:8000")
	log.Debug("auth", "password", seededSecret)
	log.Warn("token rotation", "token", seededSecret2)
	log.Error("backup failed", "reason", "repo password is "+seededSecret)
	log.Info("message contains secret " + seededSecret + " inline")

	output := buf.String()

	if strings.Contains(output, seededSecret) {
		t.Errorf("seededSecret appeared in log output:\n%s", output)
	}
	if strings.Contains(output, seededSecret2) {
		t.Errorf("seededSecret2 appeared in log output:\n%s", output)
	}
}

// TestRedactedPlaceholderPresent verifies that [REDACTED] replaces the secret.
func TestRedactedPlaceholderPresent(t *testing.T) {
	buf, log := capture(t, seededSecret)

	log.Info("auth attempt", "password", seededSecret)

	output := buf.String()
	if !strings.Contains(output, logging.Redacted) {
		t.Errorf("expected %q in output, got:\n%s", logging.Redacted, output)
	}
}

// TestSensitiveKeys verifies that attributes with sensitive key names are
// always redacted, even without an explicit secret value registered.
func TestSensitiveKeys(t *testing.T) {
	buf, log := capture(t) // no secrets registered

	log.Info("vault open",
		"password", "should-be-redacted",
		"token", "bearer-abc",
		"api_key", "key-value",
		"passphrase", "my-passphrase",
		"credential", "cred-value",
	)

	output := buf.String()
	for _, forbidden := range []string{
		"should-be-redacted",
		"bearer-abc",
		"key-value",
		"my-passphrase",
		"cred-value",
	} {
		if strings.Contains(output, forbidden) {
			t.Errorf("sensitive value %q appeared in log output:\n%s", forbidden, output)
		}
	}
}

// TestNonSensitiveValues passes through normally.
func TestNonSensitiveValues(t *testing.T) {
	buf, log := capture(t, seededSecret)

	log.Info("backup complete",
		"files_new", 42,
		"snapshot_id", "abc123",
		"repo", "rest:https://vault.lan:8000",
	)

	output := buf.String()
	if !strings.Contains(output, "backup complete") {
		t.Errorf("message 'backup complete' missing from output:\n%s", output)
	}
	if !strings.Contains(output, "abc123") {
		t.Errorf("snapshot_id should not be redacted:\n%s", output)
	}
}

// TestMessageRedaction verifies that a secret embedded in the message text is
// replaced.
func TestMessageRedaction(t *testing.T) {
	buf, log := capture(t, seededSecret)

	log.Info("initialised repo with password=" + seededSecret)

	output := buf.String()
	if strings.Contains(output, seededSecret) {
		t.Errorf("seededSecret appeared in message text:\n%s", output)
	}
	if !strings.Contains(output, logging.Redacted) {
		t.Errorf("expected %q in message, got:\n%s", logging.Redacted, output)
	}
}

// TestWithAttrs verifies that WithAttrs also redacts sensitive keys.
func TestWithAttrs(t *testing.T) {
	buf, log := capture(t, seededSecret)

	child := log.With("password", seededSecret, "component", "restic")
	child.Info("connecting")

	output := buf.String()
	if strings.Contains(output, seededSecret) {
		t.Errorf("seededSecret appeared via WithAttrs:\n%s", output)
	}
}

// TestEmptySecretList does not panic and passes non-sensitive values through.
func TestEmptySecretList(t *testing.T) {
	buf, log := capture(t)

	log.Info("normal message", "count", 5)

	output := buf.String()
	if !strings.Contains(output, "normal message") {
		t.Errorf("expected 'normal message' in output:\n%s", output)
	}
}
