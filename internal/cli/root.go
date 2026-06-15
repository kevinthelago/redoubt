package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

// Version is set at build time via -ldflags.
var Version = "dev"

// Execute runs the root command and returns any error.
func Execute() error {
	return rootCmd.Execute()
}

var rootCmd = &cobra.Command{
	Use:   "redoubt",
	Short: "LAN-only backup & recovery for your development stack",
	Long: `Redoubt is a local-network backup and recovery system.
It wraps restic with age encryption, a LAN vault, and guided key management.`,
	SilenceUsage: true,
}

func init() {
	rootCmd.AddCommand(
		versionCmd,
		keyCmd,
		NewVaultCmd(),
		backupCmd,
		NewSnapshotsCmd(),
		restoreCmd,
		scheduleCmd,
		coldCopyCmd,
		drillCmd,
		statusCmd,
		trackCmd,
	)
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the Redoubt version",
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Fprintf(cmd.OutOrStdout(), "redoubt %s\n", Version)
		return nil
	},
}

var keyCmd = &cobra.Command{
	Use:   "key",
	Short: "Manage encryption keys and break-glass escrow",
}

var backupCmd = &cobra.Command{
	Use:   "backup",
	Short: "Run a backup now",
	RunE:  stubCommand,
}


var restoreCmd = &cobra.Command{
	Use:   "restore",
	Short: "Restore files from a snapshot",
	RunE:  stubCommand,
}

var scheduleCmd = &cobra.Command{
	Use:   "schedule",
	Short: "Manage the backup schedule",
	RunE:  stubCommand,
}

var coldCopyCmd = &cobra.Command{
	Use:   "cold-copy",
	Short: "Copy snapshots to cold storage (external drive)",
	RunE:  stubCommand,
}

var drillCmd = &cobra.Command{
	Use:   "drill",
	Short: "Run a restore drill to verify backup integrity",
	RunE:  stubCommand,
}

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show backup health dashboard",
	RunE:  stubCommand,
}

var trackCmd = &cobra.Command{
	Use:   "track",
	Short: "Manage tracked sets (what to back up)",
	RunE:  stubCommand,
}

func stubCommand(cmd *cobra.Command, args []string) error {
	fmt.Fprintf(cmd.OutOrStdout(), "%s: not yet implemented\n", cmd.Use)
	return nil
}
