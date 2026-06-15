//go:build windows

package health

import (
	"fmt"
	"os/exec"
	"strings"
)

// platformNotify raises a desktop notification on Windows using PowerShell and
// the WinForms NotifyIcon API (which Windows 11 routes to the notification
// centre automatically). Non-fatal: errors are returned but callers treat them
// as advisory.
func platformNotify(title, body string) error {
	// Escape single-quotes for the PowerShell string literal.
	t := strings.ReplaceAll(title, "'", "''")
	b := strings.ReplaceAll(body, "'", "''")

	script := fmt.Sprintf(`
Add-Type -AssemblyName System.Windows.Forms
Add-Type -AssemblyName System.Drawing
$n = New-Object System.Windows.Forms.NotifyIcon
$n.Icon = [System.Drawing.SystemIcons]::Information
$n.BalloonTipTitle = '%s'
$n.BalloonTipText  = '%s'
$n.Visible = $true
$n.ShowBalloonTip(5000)
Start-Sleep -Seconds 6
$n.Dispose()
`, t, b)

	return exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-Command", script).Start()
}
