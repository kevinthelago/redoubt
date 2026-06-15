package health

import (
	"fmt"
	"time"
)

// Evaluate reads all signal states from reader and returns the aggregated health
// Status. It never returns a false green: every signal with no history grades as
// Unknown, and the overall grade is the worst of all signal grades.
//
// now is the reference time used for age calculations; pass time.Now() in
// production and a fixed time in tests.
func Evaluate(reader *StateReader, thresholds Thresholds, now time.Time) (Status, error) {
	signals := make(map[string]SignalStatus, 7)

	// --- last_backup ---
	backup, err := reader.ReadBackup()
	if err != nil {
		return Status{}, fmt.Errorf("reading backup state: %w", err)
	}
	signals[SignalLastBackup] = gradeBackup(backup, thresholds, now)

	// --- schedule ---
	sched, err := reader.ReadSchedule()
	if err != nil {
		return Status{}, fmt.Errorf("reading schedule state: %w", err)
	}
	signals[SignalSchedule] = gradeSchedule(sched)

	// --- integrity ---
	integrity, err := reader.ReadIntegrity()
	if err != nil {
		return Status{}, fmt.Errorf("reading integrity state: %w", err)
	}
	signals[SignalIntegrity] = gradeIntegrity(integrity, now)

	// --- drill ---
	drills, err := reader.ReadDrillHistory()
	if err != nil {
		return Status{}, fmt.Errorf("reading drill history: %w", err)
	}
	signals[SignalDrill] = gradeDrill(drills, thresholds, now)

	// --- cold_copy ---
	cold, err := reader.ReadColdRegistry()
	if err != nil {
		return Status{}, fmt.Errorf("reading cold registry: %w", err)
	}
	signals[SignalColdCopy] = gradeColdCopy(cold, thresholds, now)

	// --- vault ---
	vault, err := reader.ReadVault()
	if err != nil {
		return Status{}, fmt.Errorf("reading vault state: %w", err)
	}
	signals[SignalVault] = gradeVault(vault, thresholds, now)

	// --- disk ---
	diskUsed, diskErr := diskUsedPct()
	signals[SignalDisk] = gradeDisk(diskUsed, diskErr, thresholds)

	overall := worstGrade(signals)
	return Status{
		Overall:     overall,
		Signals:     signals,
		EvaluatedAt: now,
	}, nil
}

// worstGrade returns the worst (highest severity) grade across all signals.
// The order is Unknown > Critical > Warning > Healthy.
func worstGrade(signals map[string]SignalStatus) Grade {
	worst := GradeHealthy
	hasUnknown := false
	for _, s := range signals {
		if s.Grade == GradeUnknown {
			hasUnknown = true
			continue
		}
		if s.Grade > worst {
			worst = s.Grade
		}
	}
	if hasUnknown && worst == GradeHealthy {
		// Some signals have no history and none are worse than healthy.
		return GradeUnknown
	}
	return worst
}

func gradeBackup(s BackupState, t Thresholds, now time.Time) SignalStatus {
	if !s.HasHistory || s.LastBackupAt.IsZero() {
		return SignalStatus{GradeUnknown, "no backup history"}
	}
	if s.Result == "error" {
		return SignalStatus{GradeCritical, "last backup failed"}
	}
	age := now.Sub(s.LastBackupAt)
	switch {
	case age >= t.BackupCriticalAge:
		return SignalStatus{GradeCritical, fmt.Sprintf("last backup %s ago (threshold: %s)", fmtDuration(age), fmtDuration(t.BackupCriticalAge))}
	case age >= t.BackupWarnAge:
		return SignalStatus{GradeWarning, fmt.Sprintf("last backup %s ago (threshold: %s)", fmtDuration(age), fmtDuration(t.BackupWarnAge))}
	default:
		return SignalStatus{GradeHealthy, fmt.Sprintf("backed up %s ago", fmtDuration(age))}
	}
}

func gradeSchedule(s ScheduleState) SignalStatus {
	if !s.HasHistory {
		return SignalStatus{GradeUnknown, "no schedule history"}
	}
	if !s.Configured {
		return SignalStatus{GradeUnknown, "scheduler not configured"}
	}
	if s.Result == "error" {
		return SignalStatus{GradeWarning, "last scheduled run failed"}
	}
	msg := "scheduler running"
	if !s.NextRunAt.IsZero() {
		msg = fmt.Sprintf("next run %s", s.NextRunAt.Format("Mon 15:04"))
	}
	return SignalStatus{GradeHealthy, msg}
}

func gradeIntegrity(s IntegrityState, now time.Time) SignalStatus {
	if !s.HasHistory || s.LastCheckAt.IsZero() {
		return SignalStatus{GradeUnknown, "no integrity check history"}
	}
	if s.Errors > 0 {
		msg := s.Message
		if msg == "" {
			msg = fmt.Sprintf("%d error(s) found", s.Errors)
		}
		return SignalStatus{GradeCritical, msg}
	}
	age := now.Sub(s.LastCheckAt)
	return SignalStatus{GradeHealthy, fmt.Sprintf("checked %s ago", fmtDuration(age))}
}

func gradeDrill(history DrillHistory, t Thresholds, now time.Time) SignalStatus {
	if len(history) == 0 {
		return SignalStatus{GradeUnknown, "no drill history"}
	}
	// Most-recent drill is last in the slice by convention.
	last := history[len(history)-1]
	if !last.Pass {
		detail := last.Detail
		if detail == "" {
			detail = "last drill failed"
		}
		return SignalStatus{GradeCritical, detail}
	}
	age := now.Sub(last.Date)
	switch {
	case age >= t.DrillCriticalAge:
		return SignalStatus{GradeCritical, fmt.Sprintf("last drill %s ago (threshold: %s)", fmtDuration(age), fmtDuration(t.DrillCriticalAge))}
	case age >= t.DrillWarnAge:
		return SignalStatus{GradeWarning, fmt.Sprintf("last drill %s ago (threshold: %s)", fmtDuration(age), fmtDuration(t.DrillWarnAge))}
	default:
		return SignalStatus{GradeHealthy, fmt.Sprintf("last drill %s ago", fmtDuration(age))}
	}
}

func gradeColdCopy(reg ColdRegistry, t Thresholds, now time.Time) SignalStatus {
	if len(reg) == 0 {
		return SignalStatus{GradeUnknown, "no cold copy history"}
	}
	// Find most-recent copy across all registered drives.
	var latest time.Time
	for _, d := range reg {
		if d.LastCopy.After(latest) {
			latest = d.LastCopy
		}
	}
	if latest.IsZero() {
		return SignalStatus{GradeUnknown, "no cold copy history"}
	}
	age := now.Sub(latest)
	switch {
	case age >= t.ColdCopyCriticalAge:
		return SignalStatus{GradeCritical, fmt.Sprintf("last cold copy %s ago (threshold: %s)", fmtDuration(age), fmtDuration(t.ColdCopyCriticalAge))}
	case age >= t.ColdCopyWarnAge:
		return SignalStatus{GradeWarning, fmt.Sprintf("last cold copy %s ago (threshold: %s)", fmtDuration(age), fmtDuration(t.ColdCopyWarnAge))}
	default:
		return SignalStatus{GradeHealthy, fmt.Sprintf("last cold copy %s ago", fmtDuration(age))}
	}
}

func gradeVault(s VaultState, t Thresholds, now time.Time) SignalStatus {
	if !s.Configured {
		return SignalStatus{GradeUnknown, "vault not configured"}
	}
	if s.LastCheckedAt.IsZero() {
		return SignalStatus{GradeUnknown, "vault reachability never checked"}
	}
	stale := now.Sub(s.LastCheckedAt) > t.VaultCheckTTL
	if !s.Reachable {
		if stale {
			return SignalStatus{GradeWarning, fmt.Sprintf("vault unreachable (last checked %s ago)", fmtDuration(now.Sub(s.LastCheckedAt)))}
		}
		return SignalStatus{GradeCritical, "vault unreachable"}
	}
	return SignalStatus{GradeHealthy, "reachable"}
}

func gradeDisk(usedPct float64, diskErr error, t Thresholds) SignalStatus {
	if diskErr != nil {
		return SignalStatus{GradeUnknown, "disk usage unavailable"}
	}
	switch {
	case usedPct >= t.DiskCriticalPct:
		return SignalStatus{GradeCritical, fmt.Sprintf("%.0f%% used (threshold: %.0f%%)", usedPct, t.DiskCriticalPct)}
	case usedPct >= t.DiskWarnPct:
		return SignalStatus{GradeWarning, fmt.Sprintf("%.0f%% used (threshold: %.0f%%)", usedPct, t.DiskWarnPct)}
	default:
		return SignalStatus{GradeHealthy, fmt.Sprintf("%.0f%% used", usedPct)}
	}
}

// fmtDuration formats a duration in a human-friendly way (e.g. "5h", "3 days").
func fmtDuration(d time.Duration) string {
	d = d.Round(time.Minute)
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	if d < 24*time.Hour {
		return fmt.Sprintf("%dh", int(d.Hours()))
	}
	days := int(d.Hours() / 24)
	if days == 1 {
		return "1 day"
	}
	return fmt.Sprintf("%d days", days)
}
