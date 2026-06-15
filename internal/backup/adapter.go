package backup

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"

	filippage "filippo.io/age"

	internalage "github.com/kevinthelago/redoubt/internal/age"
	"github.com/kevinthelago/redoubt/internal/assets"
	"github.com/kevinthelago/redoubt/internal/keystore"
	"github.com/kevinthelago/redoubt/internal/restic"
)

// ResticAdapter adapts *restic.Runner to the backup.Runner interface.
type ResticAdapter struct {
	R *restic.Runner
}

func (a *ResticAdapter) Check(ctx context.Context) error {
	return a.R.Check(ctx, false)
}

func (a *ResticAdapter) Backup(ctx context.Context, opts RunOptions) (*BackupStats, error) {
	sum, err := a.R.Backup(ctx, opts.Sources, opts.Tags, opts.Excludes)
	if err != nil {
		return nil, err
	}
	filesTotal := int64(sum.FilesNew + sum.FilesChanged + sum.FilesUnmodified)
	return &BackupStats{
		SnapshotID: sum.SnapshotID,
		FilesNew:   int64(sum.FilesNew),
		FilesTotal: filesTotal,
		BytesAdded: sum.DataAdded,
		BytesTotal: sum.BytesProcessed,
	}, nil
}

func (a *ResticAdapter) Preview(_ context.Context, opts RunOptions) ([]string, error) {
	return opts.Sources, nil
}

// StoreResolver adapts *assets.Store to the backup.AssetResolver interface.
type StoreResolver struct {
	Store *assets.Store
}

func (r *StoreResolver) Resolve(_ context.Context) ([]TrackedAsset, error) {
	list := r.Store.List()
	out := make([]TrackedAsset, 0, len(list))
	for _, a := range list {
		ta := TrackedAsset{
			ID:       a.Path,
			Category: Category(a.Category),
			Paths:    []string{a.Path},
		}
		if a.DumpCommand != "" {
			ta.DumpCmd = strings.Fields(a.DumpCommand)
		}
		out = append(out, ta)
	}
	return out, nil
}

// KeystoreSealer seals files using the age public key loaded from the OS keystore.
// Only the public key is required — sealing is asymmetric and does NOT need the
// passphrase or an unlocked keystore session.
//
// If the keystore is not initialised (run 'redoubt key init'), SealFile returns
// a descriptive error and the secrets asset fails-soft; the rest of the backup
// continues normally.
type KeystoreSealer struct {
	once      sync.Once
	recipient filippage.Recipient
	initErr   error
}

func (s *KeystoreSealer) openRecipient() {
	s.once.Do(func() {
		km, err := keystore.Open()
		if err != nil {
			s.initErr = fmt.Errorf("keystore unavailable: %w — run 'redoubt key init'", err)
			return
		}
		r, err := filippage.ParseX25519Recipient(km.AgeRecipient())
		if err != nil {
			s.initErr = fmt.Errorf("parse age recipient: %w", err)
			return
		}
		s.recipient = r
	})
}

// SealFile encrypts src into dst using the age public key from the OS keystore.
func (s *KeystoreSealer) SealFile(_ context.Context, src, dst string) error {
	s.openRecipient()
	if s.initErr != nil {
		return s.initErr
	}

	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("open %s: %w", src, err)
	}
	defer in.Close()

	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return fmt.Errorf("create %s: %w", dst, err)
	}
	defer out.Close()

	return internalage.Seal(out, in, s.recipient)
}
