//go:build windows

package health

import (
	"fmt"
	"os/exec"
	"strings"
)

func platformNotify(title, body string) error {
	t := strings.ReplaceAll(title, "'", "''")
	b := strings.ReplaceAll(body, "'", "''")
	script := fmt.Sprintf(`
Add-Type -AssemblyName System.Windows.Forms
Add-Type -AssemblyName System.Drawing
$n = New-Object System.Windows.Forms.NotifyIcon
$n.Icon = [System.Drawing.SystemIcons]::Information
$n.Visible = $true
$n.ShowBalloonTip(5000, '%s', '%s', [System.Windows.Forms.ToolTipIcon]::Info)
Start-Sleep -Milliseconds 5500
$n.Dispose()
`, t, b)
	return exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-Command", script).Start()
}
