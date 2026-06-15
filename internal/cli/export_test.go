package cli

import (
	"io"

	"github.com/kevinthelago/redoubt/internal/snapshots"
	"github.com/spf13/cobra"
)

// ExportedBuildSnapshotsCmd exposes the internal buildSnapshotsCmd for use in
// external test packages.  Only compiled as part of the test binary.
func ExportedBuildSnapshotsCmd(newClient func(string) (snapshots.Client, error), out io.Writer) *cobra.Command {
	return buildSnapshotsCmd(newClient, out)
}
