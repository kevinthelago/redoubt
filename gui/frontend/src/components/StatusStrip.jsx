import GradeIcon from './GradeIcon.jsx'
import { WifiOffIcon } from '../icons/index.jsx'

/**
 * Always-visible bottom strip.
 * Shows: overall health grade, last backup time, LAN-only marker.
 */
export default function StatusStrip({ health, lastBackup }) {
  const grade = health?.overall ?? 'unknown'

  const gradeLabel = {
    healthy:  'All systems healthy',
    warning:  'Attention needed',
    critical: 'Action required',
    unknown:  'Not yet protected',
  }[grade] ?? 'Unknown'

  return (
    <div
      role="status"
      aria-label="System status"
      style={{
        display: 'flex',
        alignItems: 'center',
        gap: 'var(--space-4)',
        padding: '5px var(--space-4)',
        background: 'var(--color-bg-secondary)',
        borderTop: '1px solid var(--color-border)',
        fontSize: '12px',
        color: 'var(--color-text-muted)',
        flexShrink: 0,
      }}
    >
      {/* Overall grade */}
      <span style={{ display: 'flex', alignItems: 'center', gap: 'var(--space-2)' }}>
        <GradeIcon grade={grade} size={13} />
        <span style={{ color: gradeColor(grade) }}>{gradeLabel}</span>
      </span>

      {/* Last backup */}
      {lastBackup && (
        <>
          <span style={{ color: 'var(--color-border)' }}>·</span>
          <span>Last backup: {lastBackup}</span>
        </>
      )}

      {/* Spacer */}
      <span style={{ flex: 1 }} />

      {/* LAN-only badge */}
      <span style={{ display: 'flex', alignItems: 'center', gap: 'var(--space-1)', color: 'var(--color-text-muted)' }}>
        <WifiOffIcon size={12} />
        <span>LAN-only · offline</span>
      </span>
    </div>
  )
}

function gradeColor(grade) {
  return {
    healthy:  'var(--color-status-healthy)',
    warning:  'var(--color-status-warning)',
    critical: 'var(--color-status-critical)',
    unknown:  'var(--color-status-unknown)',
  }[grade] ?? 'var(--color-status-unknown)'
}
