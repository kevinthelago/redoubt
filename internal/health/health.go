// Package health exposes the DrillReporter interface for surfacing drill results.
// STUB — health stream (H1/H2) owns the full implementation.
package health

import "github.com/kevinthelago/redoubt/internal/drill"

// DrillReporter accepts drill results for aggregation into the health dashboard.
type DrillReporter interface {
	RecordDrillResult(result drill.Result) error
}
