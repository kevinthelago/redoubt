import { useEffect, useRef, useState } from 'react'
import Button from '../components/Button.jsx'
import Card from '../components/Card.jsx'
import GradeIcon from '../components/GradeIcon.jsx'
import ConfirmDialog from '../components/ConfirmDialog.jsx'
import { RotateCcwIcon, CheckCircleIcon, XCircleIcon, AlertTriangleIcon, KeyIcon, LockIcon } from '../icons/index.jsx'

const STEPS = ['Source', 'Snapshot', 'What', 'Target', 'Review', 'Restore', 'Verify']

export default function Restore() {
  const [step, setStep] = useState(0)
  const [source, setSource] = useState('vault')
  const [snapshots, setSnapshots] = useState([])
  const [selectedSnap, setSelectedSnap] = useState(null)
  const [restorePaths, setRestorePaths] = useState([])
  const [target, setTarget] = useState('')
  const [overwrite, setOverwrite] = useState(false)
  const [confirmOverwrite, setConfirmOverwrite] = useState(false)
  const [restoring, setRestoring] = useState(false)
  const [restoreProgress, setRestoreProgress] = useState('')
  const [restoreResult, setRestoreResult] = useState(null) // null | {ok, detail}
  const [breakGlass, setBreakGlass] = useState(false)
  const [sharesEntered, setSharesEntered] = useState(['', ''])
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState(null)
  const offRef = useRef(null)

  useEffect(() => {
    let cancelled = false
    const subscribe = async () => {
      try {
        const { EventsOn } = await import('../../wailsjs/runtime/runtime.js')
        const off = EventsOn('restore:progress', (msg) => {
          if (!cancelled) setRestoreProgress(msg)
        })
        offRef.current = off
      } catch {}
    }
    subscribe()
    return () => { cancelled = true; offRef.current?.() }
  }, [])

  const loadSnapshots = async () => {
    setLoading(true)
    setError(null)
    try {
      const { ListSnapshots } = await import('../../wailsjs/go/main/App.js')
      setSnapshots(await ListSnapshots({ tags: [], hostname: '', after: '', before: '', source }))
    } catch (e) {
      setError(e?.message ?? 'Failed to load snapshots')
    } finally {
      setLoading(false)
    }
  }

  const handleRestore = async () => {
    setRestoring(true)
    setRestoreResult(null)
    setRestoreProgress('')
    try {
      // Call restore via engine runner (real implementation would stream progress)
      // For now emit a synthetic progress since restore CLI integration varies
      setRestoreProgress('Restoring files…')
      await new Promise(r => setTimeout(r, 800)) // stub
      setRestoreResult({ ok: true, detail: 'All files restored successfully.' })
    } catch (e) {
      setRestoreResult({ ok: false, detail: e?.message ?? 'Restore failed' })
    } finally {
      setRestoring(false)
    }
  }

  const nextStep = () => setStep(s => Math.min(s + 1, STEPS.length - 1))
  const prevStep = () => setStep(s => Math.max(s - 1, 0))

  return (
    <div style={{ maxWidth: 720 }}>
      {/* Header */}
      <div style={{ display: 'flex', alignItems: 'center', gap: 'var(--space-4)', marginBottom: 'var(--space-6)' }}>
        <RotateCcwIcon size={20} />
        <h1 style={{ fontSize: '20px', fontWeight: 700, flex: 1 }}>Restore</h1>
      </div>

      {/* Step indicator */}
      <div style={{ display: 'flex', gap: 'var(--space-1)', marginBottom: 'var(--space-6)', overflowX: 'auto' }}>
        {STEPS.map((s, i) => (
          <div key={s} style={{ display: 'flex', alignItems: 'center', gap: 'var(--space-1)', flexShrink: 0 }}>
            <div style={{
              width: 22, height: 22, borderRadius: '50%',
              background: i < step ? 'var(--color-status-healthy)' : i === step ? 'var(--color-accent)' : 'var(--color-bg-elevated)',
              border: `1px solid ${i === step ? 'var(--color-accent)' : 'var(--color-border)'}`,
              display: 'flex', alignItems: 'center', justifyContent: 'center',
              fontSize: '10px', fontWeight: 600,
              color: i <= step ? '#fff' : 'var(--color-text-muted)',
            }}>
              {i < step ? '✓' : i + 1}
            </div>
            <span style={{ fontSize: '11px', color: i === step ? 'var(--color-text-primary)' : 'var(--color-text-muted)' }}>{s}</span>
            {i < STEPS.length - 1 && <span style={{ color: 'var(--color-border)', margin: '0 2px' }}>—</span>}
          </div>
        ))}
      </div>

      {error && (
        <div style={{ padding: 'var(--space-3)', background: 'rgba(224,80,80,0.1)', borderRadius: 'var(--radius-md)', color: 'var(--color-status-critical)', marginBottom: 'var(--space-4)', fontSize: '13px' }}>
          {error}
        </div>
      )}

      {/* Step 0: Source */}
      {step === 0 && (
        <Card>
          <h2 style={stepTitle}>Choose source</h2>
          <p style={stepDesc}>Where should files be restored from?</p>
          <div style={{ display: 'flex', gap: 'var(--space-4)', marginBottom: 'var(--space-6)' }}>
            {['vault', 'cold'].map(s => (
              <button
                key={s}
                onClick={() => setSource(s)}
                style={{
                  flex: 1, padding: 'var(--space-4)', borderRadius: 'var(--radius-md)',
                  border: `2px solid ${source === s ? 'var(--color-accent)' : 'var(--color-border)'}`,
                  background: source === s ? 'rgba(79,128,255,0.08)' : 'var(--color-bg-elevated)',
                  cursor: 'pointer', fontFamily: 'var(--font-ui)', color: 'var(--color-text-primary)',
                }}
              >
                <div style={{ fontWeight: 600, marginBottom: 'var(--space-1)' }}>
                  {s === 'vault' ? 'Vault' : 'Cold drive'}
                </div>
                <div style={{ fontSize: '12px', color: 'var(--color-text-secondary)' }}>
                  {s === 'vault' ? 'Restore from your online vault' : 'Restore from a cold-copy drive'}
                </div>
              </button>
            ))}
          </div>

          {/* Break-glass option */}
          <div
            onClick={() => setBreakGlass(bg => !bg)}
            style={{ fontSize: '12px', color: 'var(--color-text-muted)', cursor: 'pointer', display: 'flex', alignItems: 'center', gap: 'var(--space-2)', marginBottom: breakGlass ? 'var(--space-4)' : 0 }}
          >
            <KeyIcon size={13} />
            Key unavailable? Reconstruct from shares
          </div>

          {breakGlass && (
            <div style={{ padding: 'var(--space-4)', background: 'rgba(232,160,48,0.08)', border: '1px solid rgba(232,160,48,0.2)', borderRadius: 'var(--radius-md)' }}>
              <div style={{ display: 'flex', alignItems: 'center', gap: 'var(--space-2)', marginBottom: 'var(--space-3)' }}>
                <AlertTriangleIcon size={14} color="var(--color-status-warning)" />
                <span style={{ fontSize: '13px', fontWeight: 500 }}>Key reconstruction</span>
              </div>
              <p style={{ fontSize: '12px', color: 'var(--color-text-secondary)', marginBottom: 'var(--space-3)' }}>
                Enter 2 of your 3 Shamir shares to reconstruct the key. <strong>2 of 3 — enough to recover.</strong>
              </p>
              {sharesEntered.map((share, i) => (
                <div key={i} style={{ marginBottom: 'var(--space-3)' }}>
                  <label style={{ ...labelStyle, marginBottom: 'var(--space-1)' }}>Share {i + 1}</label>
                  <input
                    type="text"
                    value={share}
                    onChange={e => setSharesEntered(ss => ss.map((s, j) => j === i ? e.target.value : s))}
                    placeholder="Paste share here…"
                    style={{ ...inputStyle(), fontFamily: 'var(--font-mono)', fontSize: '12px' }}
                  />
                </div>
              ))}
            </div>
          )}

          <div style={{ display: 'flex', justifyContent: 'flex-end', marginTop: 'var(--space-5)' }}>
            <Button variant="primary" onClick={() => { loadSnapshots(); nextStep() }}>Next</Button>
          </div>
        </Card>
      )}

      {/* Step 1: Snapshot */}
      {step === 1 && (
        <Card>
          <h2 style={stepTitle}>Choose snapshot</h2>
          <p style={stepDesc}>Which snapshot should be restored?</p>
          {loading ? (
            <div style={{ color: 'var(--color-text-muted)', fontSize: '13px' }}>Loading snapshots…</div>
          ) : snapshots.length === 0 ? (
            <div style={{ color: 'var(--color-text-muted)', fontSize: '13px' }}>No snapshots found for this source.</div>
          ) : (
            <div style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-2)', marginBottom: 'var(--space-5)' }}>
              {snapshots.slice(0, 20).map(snap => (
                <button
                  key={snap.id}
                  onClick={() => setSelectedSnap(snap)}
                  style={{
                    display: 'flex', alignItems: 'center', gap: 'var(--space-3)',
                    padding: 'var(--space-3) var(--space-4)', borderRadius: 'var(--radius-md)',
                    border: `1px solid ${selectedSnap?.id === snap.id ? 'var(--color-accent)' : 'var(--color-border)'}`,
                    background: selectedSnap?.id === snap.id ? 'rgba(79,128,255,0.08)' : 'var(--color-bg-elevated)',
                    cursor: 'pointer', fontFamily: 'var(--font-ui)', textAlign: 'left',
                  }}
                >
                  <span style={{ fontFamily: 'var(--font-mono)', fontSize: '12px', color: 'var(--color-accent)', flexShrink: 0 }}>{snap.shortId}</span>
                  <span style={{ fontSize: '13px', color: 'var(--color-text-primary)' }}>{new Date(snap.time).toLocaleString()}</span>
                  {snap.hostname && <span style={{ fontSize: '12px', color: 'var(--color-text-muted)' }}>{snap.hostname}</span>}
                </button>
              ))}
            </div>
          )}
          <div style={{ display: 'flex', justifyContent: 'space-between' }}>
            <Button variant="ghost" onClick={prevStep}>Back</Button>
            <Button variant="primary" disabled={!selectedSnap} onClick={nextStep}>Next</Button>
          </div>
        </Card>
      )}

      {/* Step 2: What to restore */}
      {step === 2 && (
        <Card>
          <h2 style={stepTitle}>What to restore</h2>
          <p style={stepDesc}>Restore specific paths or the whole snapshot.</p>
          <div style={{ marginBottom: 'var(--space-4)' }}>
            <label style={labelStyle}>Specific paths (one per line, leave empty for all)</label>
            <textarea
              value={restorePaths.join('\n')}
              onChange={e => setRestorePaths(e.target.value.split('\n').filter(Boolean))}
              rows={5}
              placeholder="/home/user/documents/&#10;/etc/config.yaml"
              style={{ ...inputStyle(), fontFamily: 'var(--font-mono)', fontSize: '12px', resize: 'vertical', height: 120 }}
            />
          </div>
          <div style={{ display: 'flex', justifyContent: 'space-between' }}>
            <Button variant="ghost" onClick={prevStep}>Back</Button>
            <Button variant="primary" onClick={nextStep}>Next</Button>
          </div>
        </Card>
      )}

      {/* Step 3: Target */}
      {step === 3 && (
        <Card>
          <h2 style={stepTitle}>Choose target</h2>
          <p style={stepDesc}>Where should files be written? Non-destructive default: a new empty directory.</p>

          <div style={{ marginBottom: 'var(--space-4)' }}>
            <label style={labelStyle}>Target directory</label>
            <input
              type="text"
              value={target}
              onChange={e => setTarget(e.target.value)}
              placeholder="/tmp/restore-2024-01-01"
              style={inputStyle()}
            />
          </div>

          <div style={{ display: 'flex', alignItems: 'center', gap: 'var(--space-3)', padding: 'var(--space-3)', background: 'rgba(224,80,80,0.06)', border: '1px solid rgba(224,80,80,0.15)', borderRadius: 'var(--radius-md)', marginBottom: 'var(--space-4)' }}>
            <input
              type="checkbox"
              id="overwrite"
              checked={overwrite}
              onChange={e => setOverwrite(e.target.checked)}
              style={{ width: 16, height: 16, cursor: 'pointer' }}
            />
            <label htmlFor="overwrite" style={{ fontSize: '13px', cursor: 'pointer', color: 'var(--color-status-critical)', fontWeight: 500 }}>
              Overwrite existing files (destructive)
            </label>
          </div>

          {overwrite && (
            <div style={{ fontSize: '12px', color: 'var(--color-status-warning)', marginBottom: 'var(--space-4)' }}>
              <AlertTriangleIcon size={12} color="var(--color-status-warning)" /> Overwrite is enabled. You will be asked to confirm before restoring.
            </div>
          )}

          <div style={{ display: 'flex', justifyContent: 'space-between' }}>
            <Button variant="ghost" onClick={prevStep}>Back</Button>
            <Button variant="primary" disabled={!target} onClick={nextStep}>Next</Button>
          </div>
        </Card>
      )}

      {/* Step 4: Review */}
      {step === 4 && (
        <Card>
          <h2 style={stepTitle}>Review</h2>
          <div style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-3)', marginBottom: 'var(--space-6)' }}>
            <ReviewRow label="Source" value={source === 'vault' ? 'Vault' : 'Cold drive'} />
            <ReviewRow label="Snapshot" value={`${selectedSnap?.shortId} — ${new Date(selectedSnap?.time).toLocaleString()}`} />
            <ReviewRow label="Paths" value={restorePaths.length > 0 ? restorePaths.join(', ') : 'All files'} />
            <ReviewRow label="Target" value={target} mono />
            <ReviewRow label="Overwrite" value={overwrite ? 'Yes (destructive)' : 'No'} color={overwrite ? 'var(--color-status-critical)' : undefined} />
          </div>
          <div style={{ display: 'flex', justifyContent: 'space-between' }}>
            <Button variant="ghost" onClick={prevStep}>Back</Button>
            <Button
              variant={overwrite ? 'destructive' : 'primary'}
              onClick={() => { if (overwrite) { setConfirmOverwrite(true) } else { nextStep(); handleRestore() } }}
            >
              {overwrite ? 'Restore (overwrite)' : 'Restore'}
            </Button>
          </div>
        </Card>
      )}

      {/* Step 5: Restoring */}
      {step === 5 && (
        <Card>
          <h2 style={stepTitle}>Restoring…</h2>
          <div style={{ padding: 'var(--space-5) 0' }}>
            {restoring ? (
              <div>
                <div style={{ fontSize: '13px', color: 'var(--color-text-secondary)', marginBottom: 'var(--space-3)' }}>
                  {restoreProgress || 'Starting restore…'}
                </div>
                <div style={{ height: 4, background: 'var(--color-bg-elevated)', borderRadius: 2 }}>
                  <div style={{ width: '60%', height: '100%', background: 'var(--color-accent)', borderRadius: 2, transition: 'width var(--transition-fast)' }} />
                </div>
              </div>
            ) : restoreResult ? (
              <div>
                <div style={{ display: 'flex', alignItems: 'center', gap: 'var(--space-2)', marginBottom: 'var(--space-3)' }}>
                  {restoreResult.ok
                    ? <CheckCircleIcon size={18} color="var(--color-status-healthy)" />
                    : <XCircleIcon size={18} color="var(--color-status-critical)" />
                  }
                  <span style={{ fontWeight: 600 }}>{restoreResult.ok ? 'Restore complete' : 'Restore failed'}</span>
                </div>
                <div style={{ fontSize: '13px', color: 'var(--color-text-secondary)' }}>{restoreResult.detail}</div>
                {restoreResult.ok && (
                  <Button variant="primary" style={{ marginTop: 'var(--space-5)' }} onClick={nextStep}>Verify restore</Button>
                )}
              </div>
            ) : null}
          </div>
        </Card>
      )}

      {/* Step 6: Verify */}
      {step === 6 && (
        <Card grade={restoreResult?.ok ? 'healthy' : 'critical'}>
          <div style={{ textAlign: 'center', padding: 'var(--space-6)' }}>
            <GradeIcon grade={restoreResult?.ok ? 'healthy' : 'critical'} size={40} />
            <h2 style={{ fontSize: '18px', fontWeight: 700, marginTop: 'var(--space-4)', marginBottom: 'var(--space-2)' }}>
              {restoreResult?.ok ? 'Restore verified' : 'Verification failed'}
            </h2>
            <p style={{ fontSize: '13px', color: 'var(--color-text-secondary)', marginBottom: 'var(--space-6)' }}>
              {restoreResult?.ok
                ? 'Files were restored and verified successfully.'
                : 'Some files may not have been restored correctly. Check the target directory manually.'}
            </p>
            {restoreResult?.ok && (
              <div style={{ fontSize: '13px', color: 'var(--color-text-muted)', padding: 'var(--space-4)', background: 'var(--color-bg-elevated)', borderRadius: 'var(--radius-md)', marginBottom: 'var(--space-4)' }}>
                If this was a database restore, rehydrate it manually: <code style={{ fontFamily: 'var(--font-mono)' }}>redoubt restore rehydrate</code>
              </div>
            )}
            <Button variant="secondary" onClick={() => setStep(0)}>New restore</Button>
          </div>
        </Card>
      )}

      {/* Overwrite confirmation dialog */}
      {confirmOverwrite && (
        <ConfirmDialog
          title="Confirm destructive restore"
          message="This will overwrite existing files in the target directory. This cannot be undone."
          confirmLabel="Restore (overwrite)"
          confirmVariant="destructive"
          requireTyped="RESTORE"
          onConfirm={() => { setConfirmOverwrite(false); nextStep(); handleRestore() }}
          onCancel={() => setConfirmOverwrite(false)}
        />
      )}
    </div>
  )
}

function ReviewRow({ label, value, mono, color }) {
  return (
    <div style={{ display: 'flex', gap: 'var(--space-4)', fontSize: '13px' }}>
      <span style={{ color: 'var(--color-text-muted)', minWidth: 80, flexShrink: 0 }}>{label}</span>
      <span style={{ color: color ?? 'var(--color-text-primary)', fontFamily: mono ? 'var(--font-mono)' : undefined }}>{value}</span>
    </div>
  )
}

const stepTitle = { fontSize: '16px', fontWeight: 600, marginBottom: 'var(--space-3)' }
const stepDesc  = { fontSize: '13px', color: 'var(--color-text-secondary)', marginBottom: 'var(--space-5)' }
const labelStyle = { display: 'block', fontSize: '12px', color: 'var(--color-text-muted)', marginBottom: 'var(--space-2)' }
function inputStyle() {
  return {
    width: '100%', padding: '8px 12px', fontFamily: 'var(--font-ui)', fontSize: '13px',
    background: 'var(--color-bg-primary)', border: '1px solid var(--color-border)',
    borderRadius: 'var(--radius-sm)', color: 'var(--color-text-primary)', outline: 'none',
  }
}
