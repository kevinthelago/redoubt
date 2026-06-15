// Package logging provides a structured logger that redacts secret material
// from all output.  Every other Redoubt package must log through this package;
// callers must never construct a bare slog.Logger directly in production code.
package logging

import (
	"context"
	"io"
	"log/slog"
	"strings"
)

const Redacted = "[REDACTED]"

// sensitiveKeywords are substrings that, when found in an attribute key
// (case-insensitive), cause the entire value to be replaced with Redacted.
var sensitiveKeywords = []string{
	"password", "passwd", "secret", "token", "key", "credential", "passphrase",
}

// New returns a JSON-formatted slog.Logger that redacts any occurrence of the
// provided secret strings from message text and attribute values, and that
// always redacts attribute values whose key contains a sensitive keyword.
//
// secrets should include every secret string that could appear in log lines at
// runtime (passwords, key material, tokens).  Passing an empty list is valid
// but disables value-level secret scrubbing.
func New(w io.Writer, level slog.Level, secrets ...string) *slog.Logger {
	inner := slog.NewJSONHandler(w, &slog.HandlerOptions{Level: level})
	return slog.New(&redactingHandler{
		inner:   inner,
		secrets: filterEmpty(secrets),
	})
}

// filterEmpty removes zero-length strings from the list to avoid replacing
// every position in every string.
func filterEmpty(ss []string) []string {
	out := ss[:0]
	for _, s := range ss {
		if s != "" {
			out = append(out, s)
		}
	}
	return out
}

// redactingHandler wraps an slog.Handler and scrubs secrets before forwarding.
type redactingHandler struct {
	inner   slog.Handler
	secrets []string // literal secret values to replace
}

func (h *redactingHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.inner.Enabled(ctx, level)
}

func (h *redactingHandler) Handle(ctx context.Context, r slog.Record) error {
	clean := slog.NewRecord(r.Time, r.Level, h.scrub(r.Message), r.PC)
	r.Attrs(func(a slog.Attr) bool {
		clean.AddAttrs(h.scrubAttr(a))
		return true
	})
	return h.inner.Handle(ctx, clean)
}

func (h *redactingHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	scrubbed := make([]slog.Attr, len(attrs))
	for i, a := range attrs {
		scrubbed[i] = h.scrubAttr(a)
	}
	return &redactingHandler{inner: h.inner.WithAttrs(scrubbed), secrets: h.secrets}
}

func (h *redactingHandler) WithGroup(name string) slog.Handler {
	return &redactingHandler{inner: h.inner.WithGroup(name), secrets: h.secrets}
}

// scrubAttr returns a copy of a with the value redacted if necessary.
func (h *redactingHandler) scrubAttr(a slog.Attr) slog.Attr {
	if isSensitiveKey(a.Key) {
		return slog.String(a.Key, Redacted)
	}
	if a.Value.Kind() == slog.KindString {
		return slog.String(a.Key, h.scrub(a.Value.String()))
	}
	return a
}

// scrub replaces all occurrences of known secrets in s with Redacted.
func (h *redactingHandler) scrub(s string) string {
	for _, secret := range h.secrets {
		s = strings.ReplaceAll(s, secret, Redacted)
	}
	return s
}

// isSensitiveKey returns true when key contains a sensitive keyword
// (case-insensitive).
func isSensitiveKey(key string) bool {
	lower := strings.ToLower(key)
	for _, kw := range sensitiveKeywords {
		if strings.Contains(lower, kw) {
			return true
		}
	}
	return false
}
