package backup

import (
	"context"
	"fmt"
	"strings"

	"github.com/kevinthelago/redoubt/internal/assets"
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

// ErrSealer is a Sealer stub used until the vault stream lands age recipients.
// It returns a clear error so secrets assets fail-soft rather than being backed
// up unsealed.
type ErrSealer struct{}

func (ErrSealer) SealFile(_ context.Context, src, _ string) error {
	return fmt.Errorf("sealer not configured: vault stream required to back up secrets asset %q", src)
}
