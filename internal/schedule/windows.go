//go:build windows

package schedule

import (
	"fmt"
	"os/exec"
	"strings"
	"time"
)

const (
	taskName = "Redoubt Backup"
	psExe    = "powershell.exe"
)

func osInstall(timeHHMM, exe string) error {
	// StartWhenAvailable catches up missed runs; MultipleInstances=IgnoreNew
	// prevents concurrent executions (the backup lock handles it too, but this
	// avoids unnecessary process churn).
	script := fmt.Sprintf(
		`$a = New-ScheduledTaskAction -Execute '%s' -Argument 'schedule run';`+
			`$t = New-ScheduledTaskTrigger -Daily -At '%s';`+
			`$s = New-ScheduledTaskSettingsSet -StartWhenAvailable -MultipleInstances IgnoreNew`+
			` -ExecutionTimeLimit (New-TimeSpan -Hours 4)`+
			` -DisallowStartIfOnBatteries $false -StopIfGoingOnBatteries $false;`+
			`Register-ScheduledTask -TaskName '%s' -Action $a -Trigger $t -Settings $s -Force -ErrorAction Stop`,
		exe, timeHHMM, taskName,
	)
	return runPS(script)
}

func osRemove() error {
	// Confirm the task exists before attempting removal so we surface a clear
	// "not installed" message rather than a generic PowerShell error.
	check := fmt.Sprintf(`Get-ScheduledTask -TaskName '%s' -ErrorAction Stop`, taskName)
	if err := runPS(check); err != nil {
		return fmt.Errorf("schedule not installed")
	}
	script := fmt.Sprintf(
		`Unregister-ScheduledTask -TaskName '%s' -Confirm:$false -ErrorAction Stop`,
		taskName,
	)
	return runPS(script)
}

func osNextRun() (time.Time, error) {
	script := fmt.Sprintf(
		`(Get-ScheduledTask -TaskName '%s' -ErrorAction Stop | Get-ScheduledTaskInfo).NextRunTime | Get-Date -Format 'o'`,
		taskName,
	)
	out, err := runPSOutput(script)
	if err != nil {
		return time.Time{}, fmt.Errorf("schedule not installed")
	}
	t, err := time.Parse(time.RFC3339, strings.TrimSpace(out))
	if err != nil {
		return time.Time{}, fmt.Errorf("parse next run time %q: %w", strings.TrimSpace(out), err)
	}
	return t, nil
}

func runPS(script string) error {
	out, err := exec.Command(psExe, "-NoProfile", "-NonInteractive", "-Command", script).CombinedOutput()
	if err != nil {
		return fmt.Errorf("PowerShell: %w\n%s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func runPSOutput(script string) (string, error) {
	out, err := exec.Command(psExe, "-NoProfile", "-NonInteractive", "-Command", script).Output()
	if err != nil {
		return "", fmt.Errorf("PowerShell: %w", err)
	}
	return string(out), nil
}
