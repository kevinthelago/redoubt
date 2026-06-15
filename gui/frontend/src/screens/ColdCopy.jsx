import { useEffect, useState } from 'react'
import Button from '../components/Button.jsx'
import Card from '../components/Card.jsx'
import GradeIcon from '../components/GradeIcon.jsx'
import { HardDriveIcon, RefreshIcon, PlayIcon, CheckCircleIcon, AlertTriangleIcon } from '../icons/index.jsx'

export default function ColdCopy() {
  const [drives, setDrives] = useState([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState(null)
  const [copyingLabel, setCopyingLabel] = useState(null)
  const [copyResult, setCopyResult] = useState(null) // {label, ok, message}
  const [showDisconnectReminder, setShowDisconnectReminder] = useState(false)

  const load = async () => {
    setLoading(true)
    setError(null)
    try {
      const { ListDrives } = await import('../../wailsjs/go/main/App.js')
      setDrives(await ListDrives())
    } catch (e) {
      setError(e?.message ?? 'Failed to load drives')
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => { load() }, [])

  const handleCopy = async (label) => {
    setCopyingLabel(label)
    setCopyResult(null)
    setShowDisconnectReminder(false)
    try {
      const { MakeColdCopy } = await import('../../wailsjs/go/main/App.js')
      await MakeColdCopy(label)
      setCopyResult({ label, ok: true, message: 'Cold copy complete and verified.' })
      setShowDisconnectReminder(true)
    } catch (e) {
      setCopyResult({ label, ok: false, message: e?.message ?? 'Cold copy failed' })
    } finally {
      setCopyingLabel(null)
    }
  }

  const driveGrade = (drive) => {
    if (!drive.isMounted) return 'unknown'
    if (!drive.lastCopyTime) return 'warning'
    const hours = (Date.now() - new Date(drive.lastCopyTime)) / 3_600_000
    if (hours > 720) return 'critical' // 30 days
    if (hours > 168) return 'warning'  // 7 days
    return 'healthy'
  }

  const formatDate = (s) => s ? new Date(s).toLocaleDateString() : 'Never'

  return (
    <div style={{ maxWidth: 760 }}>
      <div style={{ display: 'flex', alignItems: 'center', gap: 'var(--space-4)', marginBottom: 'var(--space-6)' }}>
        <HardDriveIcon size={20} />
        <h1 style={{ fontSize: '20px', fontWeight: 700, flex: 1 }}>Cold Copy</h1>
        <Button variant="ghost" onClick={load} disabled={loading} icon={RefreshIcon} size="sm">Refresh</Button>
      </div>

      {error && (
        <div style={{ padding: 'var(--space-4)', background: 'rgba(224,80,80,0.1)', borderRadius: 'var(--radius-md)', color: 'var(--color-status-critical)', marginBottom: 'var(--space-4)', fontSize: '13px' }}>
          {error}
        </div>
      )}

      {/* Disconnect reminder — prominent after a copy */}
      {showDisconnectReminder && (
        <div style={{
          padding: 'var(--space-4)',
          background: 'rgba(232,160,48,0.12)',
          border: '2px solid var(--color-status-warning)',
          borderRadius: 'var(--radius-md)',
          marginBottom: 'var(--space-5)',
          display: 'flex', alignItems: 'flex-start', gap: 'var(--space-3)',
        }}>
          <AlertTriangleIcon size={20} color="var(--color-status-warning)" style={{ flexShrink: 0, marginTop: 1 }} />
          <div>
            <div style={{ fontWeight: 600, fontSize: '14px', marginBottom: 'var(--space-1)' }}>
              Disconnect the drive now
            </div>
            <div style={{ fontSize: '13px', color: 'var(--color-text-secondary)' }}>
              Cold copy is complete. Disconnect and store the drive in a separate location from your computer.
              An off-site drive protects you from local disasters.
            </div>
            <Button size="sm" variant="secondary" style={{ marginTop: 'var(--space-3)' }} onClick={() => setShowDisconnectReminder(false)}>
              Acknowledged
            </Button>
          </div>
        </div>
      )}

      {/* Copy result */}
      {copyResult && (
        <div style={{
          padding: 'var(--space-4)',
          background: copyResult.ok ? 'rgba(61,187,114,0.08)' : 'rgba(224,80,80,0.08)',
          border: `1px solid ${copyResult.ok ? 'rgba(61,187,114,0.2)' : 'rgba(224,80,80,0.2)'}`,
          borderRadius: 'var(--radius-md)', marginBottom: 'var(--space-4)',
          display: 'flex', alignItems: 'center', gap: 'var(--space-3)', fontSize: '13px',
        }}>
          {copyResult.ok
            ? <CheckCircleIcon size={15} color="var(--color-status-healthy)" />
            : <AlertTriangleIcon size={15} color="var(--color-status-critical)" />
          }
          <span>{copyResult.message}</span>
        </div>
      )}

      {loading ? (
        <div style={{ padding: 'var(--space-8)', color: 'var(--color-text-muted)' }}>Loading drives…</div>
      ) : drives.length === 0 ? (
        <EmptyState />
      ) : (
        <div style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-4)' }}>
          {drives.map(drive => {
            const grade = driveGrade(drive)
            const isCopying = copyingLabel === drive.label
            return (
              <Card key={drive.label} grade={grade}>
                <div style={{ display: 'flex', alignItems: 'flex-start', gap: 'var(--space-4)' }}>
                  <GradeIcon grade={grade} size={18} />
                  <div style={{ flex: 1 }}>
                    <div style={{ display: 'flex', alignItems: 'center', gap: 'var(--space-3)', marginBottom: 'var(--space-2)' }}>
                      <span style={{ fontWeight: 600, fontSize: '14px' }}>{drive.label}</span>
                      <span style={{
                        fontSize: '11px', padding: '2px 7px', borderRadius: 'var(--radius-sm)',
                        background: drive.isMounted ? 'rgba(61,187,114,0.12)' : 'rgba(100,112,160,0.12)',
                        color: drive.isMounted ? 'var(--color-status-healthy)' : 'var(--color-text-muted)',
                        border: `1px solid ${drive.isMounted ? 'rgba(61,187,114,0.2)' : 'rgba(100,112,160,0.2)'}`,
                      }}>
                        {drive.isMounted ? 'Mounted' : 'Not mounted'}
                      </span>
                    </div>
                    <div style={{ display: 'flex', gap: 'var(--space-5)', fontSize: '12px', color: 'var(--color-text-muted)' }}>
                      {drive.mountPath && <span>{drive.mountPath}</span>}
                      <span>Last copy: {formatDate(drive.lastCopyTime)}</span>
                      {drive.totalCopies > 0 && <span>{drive.totalCopies} total copies</span>}
                    </div>
                    {/* Staleness indicator */}
                    {grade !== 'healthy' && grade !== 'unknown' && (
                      <div style={{ marginTop: 'var(--space-2)', fontSize: '12px', color: grade === 'critical' ? 'var(--color-status-critical)' : 'var(--color-status-warning)' }}>
                        {grade === 'critical' ? 'Cold copy is more than 30 days old — make a copy now.' : 'Cold copy is more than 7 days old.'}
                      </div>
                    )}
                    {grade === 'unknown' && (
                      <div style={{ marginTop: 'var(--space-2)', fontSize: '12px', color: 'var(--color-text-muted)' }}>
                        Drive not mounted. Connect it to make a copy.
                      </div>
                    )}
                  </div>
                  <Button
                    size="sm"
                    variant="primary"
                    icon={PlayIcon}
                    onClick={() => handleCopy(drive.label)}
                    disabled={!drive.isMounted || isCopying}
                  >
                    {isCopying ? 'Copying…' : 'Copy now'}
                  </Button>
                </div>
              </Card>
            )
          })}
        </div>
      )}

      {/* What is cold copy? */}
      <div style={{ marginTop: 'var(--space-6)', padding: 'var(--space-4)', background: 'var(--color-bg-elevated)', borderRadius: 'var(--radius-md)', border: '1px solid var(--color-border-subtle)' }}>
        <div style={{ fontSize: '12px', fontWeight: 600, color: 'var(--color-text-muted)', marginBottom: 'var(--space-2)', textTransform: 'uppercase', letterSpacing: '0.04em' }}>About cold copies</div>
        <div style={{ fontSize: '13px', color: 'var(--color-text-secondary)', lineHeight: 1.6 }}>
          A cold copy is a disconnected backup on a physical drive. It protects against scenarios where your vault becomes unavailable — server failure, network loss, or accidental deletion.
          Store each cold drive in a different physical location for best protection.
        </div>
      </div>
    </div>
  )
}

function EmptyState() {
  return (
    <div style={{ padding: 'var(--space-8)', textAlign: 'center', color: 'var(--color-text-muted)' }}>
      <HardDriveIcon size={32} color="var(--color-text-muted)" style={{ marginBottom: 'var(--space-3)' }} />
      <div style={{ marginBottom: 'var(--space-2)' }}>No cold drives registered.</div>
      <div style={{ fontSize: '12px' }}>
        Register a drive using the CLI: <code style={{ fontFamily: 'var(--font-mono)' }}>redoubt cold register --label my-drive --mount /mnt/drive</code>
      </div>
    </div>
  )
}
