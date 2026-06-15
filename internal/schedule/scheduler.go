package schedule

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/kevinthelago/redoubt/internal/config"
)

// Install saves the daily run time to config and registers the OS scheduler task.
// timeHHMM must be a 24-hour "HH:MM" string (e.g. "02:00").
func Install(timeHHMM string) error {
	if err := validateTime(timeHHMM); err != nil {
		return err
	}
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("schedule install: load config: %w", err)
	}
	cfg.Schedule.Cron = timeToCron(timeHHMM)
	cfg.Schedule.Catchup = true
	if err := config.Save(cfg); err != nil {
		return fmt.Errorf("schedule install: save config: %w", err)
	}
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("schedule install: resolve executable: %w", err)
	}
	return osInstall(timeHHMM, exe)
}

// Remove unregisters the OS scheduler task.
func Remove() error {
	return osRemove()
}

// NextRun returns the next scheduled run time reported by the OS scheduler.
func NextRun() (time.Time, error) {
	return osNextRun()
}

// timeToCron converts "HH:MM" to a standard five-field cron expression.
func timeToCron(timeHHMM string) string {
	parts := strings.SplitN(timeHHMM, ":", 2)
	h, _ := strconv.Atoi(parts[0])
	m, _ := strconv.Atoi(parts[1])
	return fmt.Sprintf("%d %d * * *", m, h)
}

// validateTime checks that s is a valid 24-hour HH:MM string.
func validateTime(s string) error {
	parts := strings.SplitN(s, ":", 2)
	if len(parts) != 2 {
		return fmt.Errorf("invalid time %q: expected HH:MM (e.g. 02:00)", s)
	}
	h, errH := strconv.Atoi(parts[0])
	m, errM := strconv.Atoi(parts[1])
	if errH != nil || h < 0 || h > 23 {
		return fmt.Errorf("invalid time %q: hour must be 0-23", s)
	}
	if errM != nil || m < 0 || m > 59 {
		return fmt.Errorf("invalid time %q: minute must be 0-59", s)
	}
	return nil
}
