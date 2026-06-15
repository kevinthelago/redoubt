//go:build !windows

package health

import "os/exec"

// platformNotify raises a desktop notification on Linux (via notify-send) or
// macOS (via osascript). Non-fatal on headless systems.
func platformNotify(title, body string) error {
	// Try notify-send (Linux/freedesktop).
	if err := exec.Command("notify-send", title, body).Start(); err == nil {
		return nil
	}
	// Fall back to osascript (macOS).
	return exec.Command("osascript", "-e",
		"display notification "+shellescape(body)+" with title "+shellescape(title),
	).Start()
}

func shellescape(s string) string {
	out := make([]byte, 0, len(s)+2)
	out = append(out, '"')
	for i := 0; i < len(s); i++ {
		if s[i] == '"' || s[i] == '\\' {
			out = append(out, '\\')
		}
		out = append(out, s[i])
	}
	out = append(out, '"')
	return string(out)
}
