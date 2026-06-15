import { useEffect, useRef, useState } from 'react'
import Button from '../components/Button.jsx'
import Card from '../components/Card.jsx'
import GradeIcon from '../components/GradeIcon.jsx'
import { ArchiveIcon, PlayIcon, CheckCircleIcon, XCircleIcon, RefreshIcon } from '../icons/index.jsx'

export default function Backups() {
  const [running, setRunning] = useState(false)
  const [progress, setProgress] = useState(null) // BackupProgress object
  const [result, setResult] = useState(null)      // final BackupProgress
  const [error, setError] = useState(null)
  const [config, setConfig] = useState(null)
  const offRef = useRef(null)

  // Load config on mount.
  useEffect(() => {
    const load = async () => {
      try {
        const { GetConfig } = await import('../../wailsjs/go/main/App.js')
        setConfig(await GetConfig())
      } catch { /* ignore */ }
    }
    load()
  }, [])

  // Subscribe to backup progress events.
  useEffect(() => {
    let cancelled = false
    const subscribe = async () => {
      try {
        const { EventsOn } = await import('../../wailsjs/runtime/runtime.js')
        const off = EventsOn('backup:progress', (p) => {
          if (cancelled) return
          setProgress(p)
          if (p.done || p.error) {
            setRunning(false)
            setResult(p)
          }
        })
        offRef.current = off
      } catch { /* Wails not ready */ }
    }
    subscribe()
    return () => {
      cancelled = true
      offRef.current?.()
    }
  }, [])

  const handleBackupNow = async () => {
    setRunning(true)
    setProgress(null)
    setResult(null)
    setError(null)
    try {
      const { BackupNow } = await import('../../wailsjs/go/main/App.js')
      await BackupNow()
    } catch (e) {
      setError(e?.message ?? 'Backup failed')
      setRunning(false)
    }
  }

  const formatBytes = (b) => {
    if (!b) return '0 B'
    const units = ['B', 'KB', 'MB', 'GB', 'TB']
    let i = 0
    let v = b
    while (v >= 1024 && i < units.length - 1) { v /= 1024; i++ }
    return `${v.toFixed(1)} ${units[i]}`
  }

  return (
    <div style={{ maxWidth: 760 }}>
      <div style={{ display: 'flex', alignItems: 'center', gap: 'var(--space-4)', marginBottom: 'var(--space-6)' }}>
        <ArchiveIcon size={20} />
        <h1 style={{ fontSize: '20px', fontWeight: 700, flex: 1 }}>Backups</h1>
      </div>

      {/* Back up now */}
      <Card style={{ marginBottom: 'var(--space-5)' }}>
        <div style={{ display: 'flex', alignItems: 'flex-start', gap: 'var(--space-4)' }}>
          <div style={{ flex: 1 }}>
            <div style={{ fontWeight: 600, marginBottom: 'var(--space-1)' }}>Back up now</div>
            <div style={{ fontSize: '13px', color: 'var(--color-text-secondary)' }}>
              Runs a backup of all tracked assets to the vault.
            </div>
          </div>
          <Button
            variant="primary"
            icon={PlayIcon}
            onClick={handleBackupNow}
            disabled={running}
          >
            {running ? 'Running…' : 'Back up now'}
          </Button>
        </div>

        {/* Live progress */}
        {running && progress && (
          <div style={{ marginTop: 'var(--space-4)', padding: 'var(--space-4)', background: 'var(--color-bg-elevated)', borderRadius: 'var(--radius-md)' }}>
            <div style={{ fontSize: '13px', fontWeight: 500, marginBottom: 'var(--space-3)' }}>
              Backing up: <span style={{ fontFamily: 'var(--font-mono)' }}>{progress.asset || '…'}</span>
            </div>
            <ProgressRow label="Files" value={progress.files?.toLocaleString() ?? '0'} />
            <ProgressRow label="Data" value={formatBytes(progress.bytes)} />
            {progress.dedupeRatio > 0 && (
              <ProgressRow label="Dedupe ratio" value={`${(progress.dedupeRatio * 100).toFixed(1)}%`} />
            )}
          </div>
        )}

        {/* Running spinner */}
        {running && !progress && (
          <div style={{ marginTop: 'var(--space-4)', fontSize: '13px', color: 'var(--color-text-muted)' }}>
            Starting backup…
          </div>
        )}

        {/* Result */}
        {result && !running && (
          <div style={{
            marginTop: 'var(--space-4)',
            padding: 'var(--space-4)',
            background: result.error ? 'rgba(224,80,80,0.08)' : 'rgba(61,187,114,0.08)',
            borderRadius: 'var(--radius-md)',
            border: `1px solid ${result.error ? 'rgba(224,80,80,0.2)' : 'rgba(61,187,114,0.2)'}`,
          }}>
            <div style={{ display: 'flex', alignItems: 'center', gap: 'var(--space-2)', marginBottom: result.error ? 'var(--space-2)' : 0 }}>
              {result.error
                ? <XCircleIcon size={15} color="var(--color-status-critical)" />
                : <CheckCircleIcon size={15} color="var(--color-status-healthy)" />
              }
              <span style={{ fontWeight: 500, fontSize: '13px' }}>
                {result.error ? 'Backup failed' : 'Backup complete'}
              </span>
            </div>
            {result.error && (
              <div style={{ fontSize: '12px', color: 'var(--color-status-critical)', fontFamily: 'var(--font-mono)' }}>
                {result.error}
              </div>
            )}
            {!result.error && (
              <div style={{ display: 'flex', gap: 'var(--space-5)', marginTop: 'var(--space-2)', fontSize: '13px', color: 'var(--color-text-secondary)' }}>
                <span>{result.files?.toLocaleString()} files</span>
                <span>{formatBytes(result.bytes)}</span>
                {result.dedupeRatio > 0 && <span>{(result.dedupeRatio * 100).toFixed(1)}% deduplication</span>}
              </div>
            )}
          </div>
        )}

        {error && (
          <div style={{ marginTop: 'var(--space-3)', fontSize: '13px', color: 'var(--color-status-critical)' }}>
            {error}
          </div>
        )}
      </Card>

      {/* Schedule section */}
      <Card title="Schedule">
        {config ? (
          <ScheduleSection config={config} onSave={async (c) => {
            try {
              const { SaveConfig } = await import('../../wailsjs/go/main/App.js')
              await SaveConfig(c)
              setConfig(c)
            } catch (e) {
              setError(e?.message ?? 'Failed to save config')
            }
          }} />
        ) : (
          <div style={{ fontSize: '13px', color: 'var(--color-text-muted)' }}>Loading schedule…</div>
        )}
      </Card>
    </div>
  )
}

function ProgressRow({ label, value }) {
  return (
    <div style={{ display: 'flex', justifyContent: 'space-between', fontSize: '12px', marginBottom: 'var(--space-1)' }}>
      <span style={{ color: 'var(--color-text-muted)' }}>{label}</span>
      <span style={{ fontFamily: 'var(--font-mono)', color: 'var(--color-text-primary)' }}>{value}</span>
    </div>
  )
}

function ScheduleSection({ config, onSave }) {
  const [cron, setCron] = useState(config.scheduleCron ?? '')
  const [editing, setEditing] = useState(false)
  const [saving, setSaving] = useState(false)

  const handleSave = async () => {
    setSaving(true)
    await onSave({ ...config, scheduleCron: cron })
    setSaving(false)
    setEditing(false)
  }

  const CRON_PRESETS = [
    { label: 'Every hour',         value: '0 * * * *'     },
    { label: 'Every 6 hours',      value: '0 */6 * * *'   },
    { label: 'Daily at midnight',  value: '0 0 * * *'      },
    { label: 'Weekly (Sunday)',    value: '0 0 * * 0'      },
  ]

  return (
    <div>
      {!editing ? (
        <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
          <div>
            <div style={{ fontSize: '13px', fontFamily: 'var(--font-mono)', color: cron ? 'var(--color-text-primary)' : 'var(--color-text-muted)' }}>
              {cron || 'Not scheduled'}
            </div>
            {cron && <div style={{ fontSize: '12px', color: 'var(--color-text-muted)', marginTop: 'var(--space-1)' }}>Cron expression</div>}
          </div>
          <Button size="sm" variant="secondary" onClick={() => setEditing(true)}>Edit schedule</Button>
        </div>
      ) : (
        <div style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-3)' }}>
          <div style={{ display: 'flex', gap: 'var(--space-2)', flexWrap: 'wrap' }}>
            {CRON_PRESETS.map(p => (
              <button
                key={p.value}
                onClick={() => setCron(p.value)}
                style={{
                  padding: '4px 10px', fontSize: '12px', borderRadius: 'var(--radius-sm)',
                  border: `1px solid ${cron === p.value ? 'var(--color-accent)' : 'var(--color-border)'}`,
                  background: cron === p.value ? 'rgba(79,128,255,0.12)' : 'var(--color-bg-elevated)',
                  color: cron === p.value ? 'var(--color-accent)' : 'var(--color-text-secondary)',
                  cursor: 'pointer', fontFamily: 'var(--font-ui)',
                }}
              >
                {p.label}
              </button>
            ))}
          </div>
          <input
            type="text"
            value={cron}
            onChange={(e) => setCron(e.target.value)}
            placeholder="0 */6 * * * (cron expression)"
            style={{
              padding: '8px 12px', fontFamily: 'var(--font-mono)', fontSize: '13px',
              background: 'var(--color-bg-primary)', border: '1px solid var(--color-border)',
              borderRadius: 'var(--radius-sm)', color: 'var(--color-text-primary)', outline: 'none', width: '100%',
            }}
          />
          <div style={{ display: 'flex', gap: 'var(--space-3)', justifyContent: 'flex-end' }}>
            <Button size="sm" variant="ghost" onClick={() => setEditing(false)}>Cancel</Button>
            <Button size="sm" variant="primary" disabled={saving} onClick={handleSave}>
              {saving ? 'Saving…' : 'Save'}
            </Button>
          </div>
        </div>
      )}
    </div>
  )
}
