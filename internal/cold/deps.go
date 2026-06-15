package cold

import "context"

// Backend is the interface this package requires from the restic layer.
//
// It is intentionally minimal: the restic wrapper (internal/restic) owns
// credential management and the vault URL. The Engine drives high-level
// operations only. Both the cold repository and the vault share the same
// key material so that restic copy can transfer raw pack data directly.
//
// The implementing type in internal/restic satisfies this interface without
// importing this package — Go's implicit interface satisfaction avoids an
// import cycle. The main command wires them together.
type Backend interface {
	// InitLocalRepo creates a new restic repository at localPath using the
	// same key material as the vault.
	InitLocalRepo(ctx context.Context, localPath string) error

	// IsLocalRepoReady reports whether localPath is an initialised,
	// accessible restic repository reachable with the current credentials.
	IsLocalRepoReady(ctx context.Context, localPath string) (bool, error)

	// CopySnapshotsTo copies every snapshot that is present in the vault
	// but absent from the repository at localPath.
	// It is idempotent: running it twice copies nothing on the second pass.
	CopySnapshotsTo(ctx context.Context, localPath string) error

	// CheckLocalRepo runs restic's structural integrity check on the
	// repository at localPath.
	CheckLocalRepo(ctx context.Context, localPath string) error

	// ForgetInLocalRepo applies the given retention policy and prunes the
	// repository at localPath.
	// Zero values in RetentionPolicy mean "no limit" for that dimension.
	ForgetInLocalRepo(ctx context.Context, localPath string, p RetentionPolicy) error
}

// ConfigProvider supplies cold-copy settings and the path for the drive
// registry. The internal/config package implements this interface.
type ConfigProvider interface {
	// ColdConfig returns the active cold-copy configuration.
	ColdConfig() ColdConfig

	// DriveRegistryPath returns the filesystem path for the cold-drive
	// registry TOML file. The parent directory must already exist.
	DriveRegistryPath() string
}
