package config

import (
	"os"
	"path/filepath"
	"runtime"
)

// DataDir returns the OS-specific directory where Redoubt stores runtime data
// (DB, status files, cached state).
//
// Windows: %APPDATA%\Redoubt
// macOS:   ~/Library/Application Support/Redoubt
// Linux:   $XDG_DATA_HOME/redoubt or ~/.local/share/redoubt
func DataDir() string {
	switch runtime.GOOS {
	case "windows":
		if d := os.Getenv("APPDATA"); d != "" {
			return filepath.Join(d, "Redoubt")
		}
		return filepath.Join(os.Getenv("USERPROFILE"), "AppData", "Roaming", "Redoubt")
	case "darwin":
		home, _ := os.UserHomeDir()
		return filepath.Join(home, "Library", "Application Support", "Redoubt")
	default:
		if d := os.Getenv("XDG_DATA_HOME"); d != "" {
			return filepath.Join(d, "redoubt")
		}
		home, _ := os.UserHomeDir()
		return filepath.Join(home, ".local", "share", "redoubt")
	}
}

// ConfigDir returns the OS-specific directory where Redoubt stores its
// configuration.
//
// Windows: %APPDATA%\Redoubt  (same as DataDir — Windows convention)
// macOS:   ~/Library/Application Support/Redoubt
// Linux:   $XDG_CONFIG_HOME/redoubt or ~/.config/redoubt
func ConfigDir() string {
	switch runtime.GOOS {
	case "windows":
		if d := os.Getenv("APPDATA"); d != "" {
			return filepath.Join(d, "Redoubt")
		}
		return filepath.Join(os.Getenv("USERPROFILE"), "AppData", "Roaming", "Redoubt")
	case "darwin":
		home, _ := os.UserHomeDir()
		return filepath.Join(home, "Library", "Application Support", "Redoubt")
	default:
		if d := os.Getenv("XDG_CONFIG_HOME"); d != "" {
			return filepath.Join(d, "redoubt")
		}
		home, _ := os.UserHomeDir()
		return filepath.Join(home, ".config", "redoubt")
	}
}

// ConfigPath returns the full filesystem path of the main configuration file.
func ConfigPath() string {
	return filepath.Join(ConfigDir(), "config.toml")
}
