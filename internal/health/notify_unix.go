//go:build !windows

package health

import (
	"fmt"
	"os/exec"
)

func platformNotify(title, body string) error {
	if err := exec.Command("notify-send", title, body).Start(); err == nil {
		return nil
	}
	return exec.Command("osascript", "-e",
		fmt.Sprintf("display notification %q with title %q", body, title),
	).Start()
}
