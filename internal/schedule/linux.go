//go:build linux

package schedule

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	svcName    = "redoubt-backup"
	unitSubdir = ".config/systemd/user"
)

func unitDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home dir: %w", err)
	}
	return filepath.Join(home, unitSubdir), nil
}

func osInstall(timeHHMM, exe string) error {
	dir, err := unitDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create systemd unit dir: %w", err)
	}

	service := fmt.Sprintf(
		"[Unit]\nDescription=Redoubt scheduled backup\n\n[Service]\nType=oneshot\nExecStart=%s schedule run\n",
		exe,
	)
	// Persistent=true catches up any missed fires since the last machine boot.
	timer := fmt.Sprintf(
		"[Unit]\nDescription=Redoubt backup timer\n\n[Install]\nWantedBy=timers.target\n\n[Timer]\nOnCalendar=*-*-* %s:00\nPersistent=true\nUnit=%s.service\n",
		timeHHMM, svcName,
	)

	if err := os.WriteFile(filepath.Join(dir, svcName+".service"), []byte(service), 0o644); err != nil {
		return fmt.Errorf("write service unit: %w", err)
	}
	if err := os.WriteFile(filepath.Join(dir, svcName+".timer"), []byte(timer), 0o644); err != nil {
		return fmt.Errorf("write timer unit: %w", err)
	}
	if err := systemctl("daemon-reload"); err != nil {
		return err
	}
	return systemctl("enable", "--now", svcName+".timer")
}

func osRemove() error {
	_ = systemctl("disable", "--now", svcName+".timer")
	dir, _ := unitDir()
	if dir != "" {
		_ = os.Remove(filepath.Join(dir, svcName+".service"))
		_ = os.Remove(filepath.Join(dir, svcName+".timer"))
	}
	return systemctl("daemon-reload")
}

func osNextRun() (time.Time, error) {
	out, err := exec.Command(
		"systemctl", "--user", "show", svcName+".timer",
		"--property=NextElapseUSecRealtime", "--value",
	).Output()
	if err != nil {
		return time.Time{}, fmt.Errorf("schedule not installed")
	}
	usStr := strings.TrimSpace(string(out))
	us, err := strconv.ParseInt(usStr, 10, 64)
	if err != nil || us == 0 {
		return time.Time{}, fmt.Errorf("schedule not installed or not yet active")
	}
	return time.Unix(us/1_000_000, (us%1_000_000)*1_000).UTC(), nil
}

func systemctl(args ...string) error {
	cmd := exec.Command("systemctl", append([]string{"--user"}, args...)...)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("systemctl --user %s: %w\n%s",
			strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return nil
}
