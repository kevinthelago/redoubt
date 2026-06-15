import { useEffect, useState } from 'react'
import Button from '../components/Button.jsx'
import Card from '../components/Card.jsx'
import { SettingsIcon, SunIcon, MoonIcon, WifiOffIcon } from '../icons/index.jsx'

export default function Settings({ theme, onThemeChange }) {
  const [config, setConfig] = useState(null)
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState(null)
  const [saved, setSaved] = useState(false)

  // Threshold form state
  const [maxHours, setMaxHours] = useState(24)
  const [maxRepoGB, setMaxRepoGB] = useState(50)

  const load = async () => {
    setLoading(true)
    setError(null)
    try {
      const { GetConfig } = await import('../../wailsjs/go/main/App.js')
      const cfg = await GetConfig()
      setConfig(cfg)
      setMaxHours(cfg.thresholds?.maxHoursWithoutBackup ?? 24)
      setMaxRepoGB(cfg.thresholds?.maxRepoSizeGB ?? 50)
    } catch (e) {
      setError(e?.message ?? 'Failed to load config')
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => { load() }, [])

  const handleSave = async () => {
    if (!config) return
    setSaving(true)
    setSaved(false)
    setError(null)
    try {
      const updated = {
        ...config,
        thresholds: {
          maxHoursWithoutBackup: Number(maxHours),
          maxRepoSizeGB: Number(maxRepoGB),
        },
      }
      const { SaveConfig } = await import('../../wailsjs/go/main/App.js')
      await SaveConfig(updated)
      setConfig(updated)
      setSaved(true)
      setTimeout(() => setSaved(false), 2000)
    } catch (e) {
      setError(e?.message ?? 'Failed to save config')
    } finally {
      setSaving(false)
    }
  }

  return (
    <div style={{ maxWidth: 640 }}>
      <div style={{ display: 'flex', alignItems: 'center', gap: 'var(--space-4)', marginBottom: 'var(--space-6)' }}>
        <SettingsIcon size={20} />
        <h1 style={{ fontSize: '20px', fontWeight: 700, flex: 1 }}>Settings</h1>
      </div>

      {error && (
        <div style={{ padding: 'var(--space-4)', background: 'rgba(224,80,80,0.1)', borderRadius: 'var(--radius-md)', color: 'var(--color-status-critical)', marginBottom: 'var(--space-4)', fontSize: '13px' }}>
          {error}
        </div>
      )}

      {/* Theme */}
      <Card title="Appearance" style={{ marginBottom: 'var(--space-5)' }}>
        <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
          <div>
            <div style={{ fontSize: '13px', fontWeight: 500, marginBottom: 'var(--space-1)' }}>Theme</div>
            <div style={{ fontSize: '12px', color: 'var(--color-text-muted)' }}>
              {theme === 'dark' ? 'Dark theme (default)' : 'Light theme'}
            </div>
          </div>
          <div style={{ display: 'flex', gap: 'var(--space-2)' }}>
            <button
              onClick={() => onThemeChange('dark')}
              title="Dark theme"
              aria-label="Dark theme"
              aria-pressed={theme === 'dark'}
              style={{
                display: 'flex', alignItems: 'center', gap: 'var(--space-2)',
                padding: '6px 12px', borderRadius: 'var(--radius-sm)',
                border: `1px solid ${theme === 'dark' ? 'var(--color-accent)' : 'var(--color-border)'}`,
                background: theme === 'dark' ? 'rgba(79,128,255,0.12)' : 'var(--color-bg-elevated)',
                color: theme === 'dark' ? 'var(--color-accent)' : 'var(--color-text-secondary)',
                cursor: 'pointer', fontFamily: 'var(--font-ui)', fontSize: '12px',
              }}
            >
              <MoonIcon size={13} />
              Dark
            </button>
            <button
              onClick={() => onThemeChange('light')}
              title="Light theme"
              aria-label="Light theme"
              aria-pressed={theme === 'light'}
              style={{
                display: 'flex', alignItems: 'center', gap: 'var(--space-2)',
                padding: '6px 12px', borderRadius: 'var(--radius-sm)',
                border: `1px solid ${theme === 'light' ? 'var(--color-accent)' : 'var(--color-border)'}`,
                background: theme === 'light' ? 'rgba(79,128,255,0.12)' : 'var(--color-bg-elevated)',
                color: theme === 'light' ? 'var(--color-accent)' : 'var(--color-text-secondary)',
                cursor: 'pointer', fontFamily: 'var(--font-ui)', fontSize: '12px',
              }}
            >
              <SunIcon size={13} />
              Light
            </button>
          </div>
        </div>
      </Card>

      {/* Thresholds */}
      {loading ? (
        <Card title="Thresholds" style={{ marginBottom: 'var(--space-5)' }}>
          <div style={{ color: 'var(--color-text-muted)', fontSize: '13px' }}>Loading…</div>
        </Card>
      ) : (
        <Card title="Thresholds" style={{ marginBottom: 'var(--space-5)' }}>
          <p style={{ fontSize: '13px', color: 'var(--color-text-secondary)', marginBottom: 'var(--space-4)' }}>
            Configure when backup signals change from healthy to warning or critical.
          </p>
          <div style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-4)' }}>
            <div>
              <label style={labelStyle}>Backup staleness threshold (hours)</label>
              <div style={{ display: 'flex', alignItems: 'center', gap: 'var(--space-3)' }}>
                <input
                  type="number"
                  min={1}
                  max={8760}
                  value={maxHours}
                  onChange={e => setMaxHours(e.target.value)}
                  style={{ ...inputStyle(), width: 120 }}
                />
                <span style={{ fontSize: '12px', color: 'var(--color-text-muted)' }}>
                  Warning after {maxHours} hour{maxHours !== 1 ? 's' : ''} without a backup
                </span>
              </div>
            </div>
            <div>
              <label style={labelStyle}>Maximum repo size (GB)</label>
              <div style={{ display: 'flex', alignItems: 'center', gap: 'var(--space-3)' }}>
                <input
                  type="number"
                  min={1}
                  max={10000}
                  value={maxRepoGB}
                  onChange={e => setMaxRepoGB(e.target.value)}
                  style={{ ...inputStyle(), width: 120 }}
                />
                <span style={{ fontSize: '12px', color: 'var(--color-text-muted)' }}>
                  Warning when repo exceeds {maxRepoGB} GB
                </span>
              </div>
            </div>
          </div>
          <div style={{ display: 'flex', alignItems: 'center', gap: 'var(--space-3)', marginTop: 'var(--space-5)' }}>
            <Button variant="primary" onClick={handleSave} disabled={saving}>
              {saving ? 'Saving…' : 'Save thresholds'}
            </Button>
            {saved && (
              <span style={{ fontSize: '13px', color: 'var(--color-status-healthy)' }}>Saved</span>
            )}
          </div>
        </Card>
      )}

      {/* About / offline guarantee */}
      <Card title="About Redoubt" style={{ marginBottom: 'var(--space-5)' }}>
        <div style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-4)' }}>
          <div style={{
            display: 'flex', alignItems: 'flex-start', gap: 'var(--space-3)',
            padding: 'var(--space-4)',
            background: 'var(--color-bg-elevated)',
            borderRadius: 'var(--radius-md)',
            border: '1px solid var(--color-border-subtle)',
          }}>
            <WifiOffIcon size={18} color="var(--color-text-muted)" style={{ flexShrink: 0, marginTop: 2 }} />
            <div>
              <div style={{ fontWeight: 600, fontSize: '13px', marginBottom: 'var(--space-1)' }}>
                Fully offline — no WAN connections
              </div>
              <div style={{ fontSize: '13px', color: 'var(--color-text-secondary)', lineHeight: 1.6 }}>
                Redoubt never connects to the internet. There are no analytics, no telemetry, no external CDN
                dependencies, and no remote services except your own vault on your own LAN.
                All backup data stays on hardware you control.
              </div>
            </div>
          </div>

          <VersionRow label="GUI version" value="1.0.0" />
          <VersionRowAsync label="Engine version" />
        </div>
      </Card>

      {/* Print styles for settings page */}
      <style>{`
        @media print {
          .no-print { display: none !important; }
        }
      `}</style>
    </div>
  )
}

function VersionRow({ label, value }) {
  return (
    <div style={{ display: 'flex', justifyContent: 'space-between', fontSize: '13px' }}>
      <span style={{ color: 'var(--color-text-muted)' }}>{label}</span>
      <span style={{ fontFamily: 'var(--font-mono)', color: 'var(--color-text-secondary)' }}>{value}</span>
    </div>
  )
}

function VersionRowAsync({ label }) {
  const [version, setVersion] = useState('…')

  useEffect(() => {
    const load = async () => {
      try {
        setVersion('redoubt engine')
      } catch {
        setVersion('unknown')
      }
    }
    load()
  }, [])

  return <VersionRow label={label} value={version} />
}

const labelStyle = {
  display: 'block',
  fontSize: '12px',
  color: 'var(--color-text-muted)',
  marginBottom: 'var(--space-2)',
}

function inputStyle() {
  return {
    padding: '8px 12px',
    fontFamily: 'var(--font-ui)',
    fontSize: '13px',
    background: 'var(--color-bg-primary)',
    border: '1px solid var(--color-border)',
    borderRadius: 'var(--radius-sm)',
    color: 'var(--color-text-primary)',
    outline: 'none',
  }
}
