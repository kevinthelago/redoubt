package vault

import "errors"

var (
	// ErrPublicAddress is returned when a bind address is not a private RFC1918 LAN address.
	// Loopback (127.x, ::1) is also refused — the vault must be reachable from other LAN nodes.
	ErrPublicAddress = errors.New("vault: address must be a private LAN (RFC1918) address — public and loopback addresses are refused")

	// ErrPortInUse is returned when the requested port is already bound.
	ErrPortInUse = errors.New("vault: port already in use")

	// ErrPathNotWritable is returned when the repository path cannot be created or written to.
	ErrPathNotWritable = errors.New("vault: repository path is not writable")

	// ErrAlreadyInited is returned when vault init is called and a repository already exists.
	// This is non-fatal: callers can inspect the existing profile and continue.
	ErrAlreadyInited = errors.New("vault: repository already initialized — reusing existing repo")

	// ErrVaultUnreachable is returned when GetStatus cannot contact the vault within its timeout.
	ErrVaultUnreachable = errors.New("vault: host unreachable")

	// ErrResticNotFound is returned when the restic binary cannot be located on PATH.
	ErrResticNotFound = errors.New("vault: restic binary not found on PATH — install restic from https://restic.net")

	// ErrRestServerNotFound is returned when rest-server binary cannot be located on PATH.
	ErrRestServerNotFound = errors.New("vault: rest-server binary not found on PATH — install rest-server from https://github.com/restic/rest-server")
)
