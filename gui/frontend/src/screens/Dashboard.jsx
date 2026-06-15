import { useEffect, useState } from 'react'
import Card from '../components/Card.jsx'
import Button from '../components/Button.jsx'
import GradeIcon from '../components/GradeIcon.jsx'
import {
  ShieldIcon, ArchiveIcon, CameraIcon, RotateCcwIcon, HardDriveIcon,
  CheckCircleIcon, KeyIcon, ServerIcon, PlayIcon, ArrowRightIcon,
} from '../icons/index.jsx'

const SIGNAL_META = {
  last_backup: { label: 'Last Backup',    Icon: ArchiveIcon,      screen: 'backups'   },
  schedule:    { label: 'Schedule',       Icon: CameraIcon,       screen: 'backups'   },
  integrity:   { label: 'Integrity',      Icon: ShieldIcon,       screen: 'snapshots' },
  drill:       { label: 'Restore Drills', Icon: CheckCircleIcon,  screen: 'drills'    },
  cold_copy:   { label: 'Cold Copy',      Icon: HardDriveIcon,    screen: 'cold'      },
  vault:       { label: 'Vault',          Icon: ServerIcon,       screen: 'vault'     },
  disk:        { label: 'Disk Space',     Icon: HardDriveIcon,    screen: 'settings'  },
}

const ONBOARDING_STEPS = [
  { id: 'keys',    label: 'Set up encryption key', screen: 'keys'     },
  { id: 'vault',   label: 'Connect vault',         screen: 'vault'    },
  { id: 'assets',  label: 'Add assets to protect', screen: 'assets'   },
  { id: 'backups', label: 'Run first backup',       screen: 'backups'  },
]

export default function Dashboard({ onNavigate, health }) {
  const [status, setStatus] = useState(health)

  // Keep local copy in sync.
  useEffect(() => { setStatus(health) }, [health])

  // Refresh on mount if health not yet loaded.
  useEffect(() => {
    if (health) return
    let cancelled = false
    const load = async () => {
      try {
        const { GetStatus } = await import('../../wailsjs/go/main/App.js')
        const s = await GetStatus()
        if (!cancelled) setStatus(s)
      } catch { /* ignore */ }
    }
    load()
    return () => { cancelled = true }
  }, [health])

  const grade = status?.overall ?? 'unknown'
  const signals = status?.signals ?? {}
  const isFirstRun = grade === 'unknown' && Object.keys(signals).length === 0

  return (
    <div style={{ maxWidth: 900, margin: '0 auto' }}>
      {/* Hero */}
      <OverallStatus grade={grade} onNavigate={onNavigate} />

      {isFirstRun ? (
        <OnboardingChecklist onNavigate={onNavigate} />
      ) : (
        <SignalGrid signals={signals} onNavigate={onNavigate} />
      )}
    </div>
  )
}

function OverallStatus({ grade, onNavigate }) {
  const config = {
    healthy:  {
      heading: 'Your data is protected',
      sub: 'All backup signals are healthy.',
      cta: null,
    },
    warning:  {
      heading: 'Attention needed',
      sub: 'One or more signals need attention.',
      cta: { label: 'View details', screen: 'health' },
    },
    critical: {
      heading: 'Action required',
      sub: 'A critical backup signal requires immediate action.',
      cta: { label: 'View details', screen: 'health' },
    },
    unknown:  {
      heading: 'Not yet protected',
      sub: 'Complete setup to start protecting your data.',
      cta: { label: 'Get protected', screen: 'keys' },
    },
  }[grade] ?? { heading: 'Status unknown', sub: '', cta: null }

  const gradeColor = {
    healthy:  'var(--color-status-healthy)',
    warning:  'var(--color-status-warning)',
    critical: 'var(--color-status-critical)',
    unknown:  'var(--color-status-unknown)',
  }[grade]

  return (
    <div style={{
      background: 'var(--color-bg-secondary)',
      border: '1px solid var(--color-border)',
      borderRadius: 'var(--radius-lg)',
      padding: 'var(--space-8)',
      marginBottom: 'var(--space-6)',
      display: 'flex',
      alignItems: 'center',
      gap: 'var(--space-6)',
    }}>
      <div style={{
        width: 64, height: 64, borderRadius: '50%',
        background: `${gradeColor}18`,
        border: `2px solid ${gradeColor}40`,
        display: 'flex', alignItems: 'center', justifyContent: 'center',
        flexShrink: 0,
      }}>
        <GradeIcon grade={grade} size={32} />
      </div>
      <div style={{ flex: 1 }}>
        <h1 style={{ fontSize: '22px', fontWeight: 700, marginBottom: 'var(--space-1)' }}>
          {config.heading}
        </h1>
        <p style={{ color: 'var(--color-text-secondary)', fontSize: '14px' }}>{config.sub}</p>
      </div>
      <div style={{ display: 'flex', gap: 'var(--space-3)', flexShrink: 0 }}>
        {config.cta && (
          <Button variant="primary" onClick={() => onNavigate(config.cta.screen)} icon={ArrowRightIcon}>
            {config.cta.label}
          </Button>
        )}
        <Button variant="secondary" onClick={() => onNavigate('backups')} icon={PlayIcon}>
          Back up now
        </Button>
        <Button variant="ghost" onClick={() => onNavigate('restore')} icon={RotateCcwIcon}>
          Restore
        </Button>
      </div>
    </div>
  )
}

function SignalGrid({ signals, onNavigate }) {
  const keys = Object.keys(SIGNAL_META)
  return (
    <div style={{
      display: 'grid',
      gridTemplateColumns: 'repeat(auto-fill, minmax(260px, 1fr))',
      gap: 'var(--space-4)',
    }}>
      {keys.map((key) => {
        const meta = SIGNAL_META[key]
        const sig = signals[key]
        if (!meta) return null
        const grade = sig?.grade ?? 'unknown'
        return (
          <Card
            key={key}
            grade={grade}
            onClick={() => onNavigate(meta.screen)}
          >
            <div style={{ display: 'flex', alignItems: 'flex-start', gap: 'var(--space-3)' }}>
              <meta.Icon size={16} color="var(--color-text-muted)" />
              <div style={{ flex: 1, minWidth: 0 }}>
                <div style={{ fontSize: '12px', color: 'var(--color-text-muted)', fontWeight: 500, marginBottom: 'var(--space-1)' }}>
                  {meta.label}
                </div>
                <div style={{ display: 'flex', alignItems: 'center', gap: 'var(--space-2)', marginBottom: 'var(--space-1)' }}>
                  <GradeIcon grade={grade} size={13} showLabel />
                </div>
                {sig?.message && (
                  <div style={{ fontSize: '12px', color: 'var(--color-text-secondary)', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                    {sig.message}
                  </div>
                )}
              </div>
            </div>
          </Card>
        )
      })}
    </div>
  )
}

function OnboardingChecklist({ onNavigate }) {
  return (
    <div style={{
      background: 'var(--color-bg-secondary)',
      border: '1px solid var(--color-border)',
      borderRadius: 'var(--radius-lg)',
      padding: 'var(--space-6)',
    }}>
      <h2 style={{ fontSize: '15px', fontWeight: 600, marginBottom: 'var(--space-2)' }}>Get protected in 4 steps</h2>
      <p style={{ fontSize: '13px', color: 'var(--color-text-secondary)', marginBottom: 'var(--space-5)' }}>
        Complete these steps to start protecting your data.
      </p>
      <ol style={{ listStyle: 'none', display: 'flex', flexDirection: 'column', gap: 'var(--space-3)' }}>
        {ONBOARDING_STEPS.map((step, i) => (
          <li key={step.id}>
            <button
              onClick={() => onNavigate(step.screen)}
              style={{
                display: 'flex', alignItems: 'center', gap: 'var(--space-4)',
                width: '100%', padding: 'var(--space-3) var(--space-4)',
                background: 'var(--color-bg-elevated)',
                border: '1px solid var(--color-border)',
                borderRadius: 'var(--radius-md)',
                cursor: 'pointer', color: 'var(--color-text-primary)',
                fontFamily: 'var(--font-ui)', fontSize: '13px', textAlign: 'left',
              }}
            >
              <span style={{
                width: 24, height: 24, borderRadius: '50%',
                background: 'var(--color-bg-primary)',
                border: '1px solid var(--color-border)',
                display: 'flex', alignItems: 'center', justifyContent: 'center',
                fontSize: '11px', fontWeight: 600, color: 'var(--color-text-muted)', flexShrink: 0,
              }}>{i + 1}</span>
              <span style={{ flex: 1 }}>{step.label}</span>
              <ArrowRightIcon size={14} color="var(--color-text-muted)" />
            </button>
          </li>
        ))}
      </ol>
    </div>
  )
}
