import { useEffect, useState } from 'react'
import Button from '../components/Button.jsx'
import Card from '../components/Card.jsx'
import GradeIcon from '../components/GradeIcon.jsx'
import { CheckCircleIcon, XCircleIcon, RefreshIcon, PlayIcon } from '../icons/index.jsx'

export default function Drills() {
  const [history, setHistory] = useState([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState(null)
  const [running, setRunning] = useState(false)
  const [runResult, setRunResult] = useState(null) // {pass, detail}

  const load = async () => {
    setLoading(true)
    setError(null)
    try {
      const { GetDrillHistory } = await import('../../wailsjs/go/main/App.js')
      setHistory(await GetDrillHistory())
    } catch (e) {
      setError(e?.message ?? 'Failed to load drill history')
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => { load() }, [])

  const handleRun = async () => {
    setRunning(true)
    setRunResult(null)
    try {
      const { RunDrill } = await import('../../wailsjs/go/main/App.js')
      await RunDrill()
      setRunResult({ pass: true, detail: 'Restore drill completed successfully. Data verified.' })
      await load()
    } catch (e) {
      setRunResult({ pass: false, detail: e?.message ?? 'Drill failed' })
      await load()
    } finally {
      setRunning(false)
    }
  }

  // A failed drill is a prominent warning.
  const lastResult = history[0]
  const lastFailed = lastResult && !lastResult.pass

  return (
    <div style={{ maxWidth: 760 }}>
      <div style={{ display: 'flex', alignItems: 'center', gap: 'var(--space-4)', marginBottom: 'var(--space-6)' }}>
        <CheckCircleIcon size={20} />
        <h1 style={{ fontSize: '20px', fontWeight: 700, flex: 1 }}>Restore Drills</h1>
        <Button variant="ghost" onClick={load} disabled={loading} icon={RefreshIcon} size="sm">Refresh</Button>
        <Button variant="primary" icon={PlayIcon} onClick={handleRun} disabled={running}>
          {running ? 'Running drill…' : 'Run drill now'}
        </Button>
      </div>

      {/* Prominent failure banner */}
      {lastFailed && (
        <div style={{
          padding: 'var(--space-4)',
          background: 'rgba(224,80,80,0.12)',
          border: '2px solid var(--color-status-critical)',
          borderRadius: 'var(--radius-md)',
          marginBottom: 'var(--space-5)',
          display: 'flex', alignItems: 'flex-start', gap: 'var(--space-3)',
        }}>
          <XCircleIcon size={20} color="var(--color-status-critical)" style={{ flexShrink: 0, marginTop: 1 }} />
          <div>
            <div style={{ fontWeight: 700, fontSize: '14px', color: 'var(--color-status-critical)', marginBottom: 'var(--space-1)' }}>
              Last drill FAILED
            </div>
            <div style={{ fontSize: '13px', color: 'var(--color-text-secondary)' }}>
              {lastResult.detail || 'The restore drill did not complete successfully. Investigate before relying on this backup.'}
            </div>
            <div style={{ fontSize: '12px', color: 'var(--color-text-muted)', marginTop: 'var(--space-2)' }}>
              {lastResult.asset} — {new Date(lastResult.date).toLocaleDateString()}
            </div>
          </div>
        </div>
      )}

      {error && (
        <div style={{ padding: 'var(--space-4)', background: 'rgba(224,80,80,0.1)', borderRadius: 'var(--radius-md)', color: 'var(--color-status-critical)', marginBottom: 'var(--space-4)', fontSize: '13px' }}>
          {error}
        </div>
      )}

      {/* Run result */}
      {runResult && (
        <div style={{
          padding: 'var(--space-4)',
          background: runResult.pass ? 'rgba(61,187,114,0.08)' : 'rgba(224,80,80,0.08)',
          border: `1px solid ${runResult.pass ? 'rgba(61,187,114,0.2)' : 'rgba(224,80,80,0.2)'}`,
          borderRadius: 'var(--radius-md)', marginBottom: 'var(--space-4)',
          display: 'flex', alignItems: 'flex-start', gap: 'var(--space-3)',
        }}>
          {runResult.pass
            ? <CheckCircleIcon size={16} color="var(--color-status-healthy)" />
            : <XCircleIcon size={16} color="var(--color-status-critical)" />
          }
          <div>
            <div style={{ fontWeight: 600, fontSize: '13px', marginBottom: 'var(--space-1)' }}>
              {runResult.pass ? 'Drill passed' : 'Drill failed'}
            </div>
            <div style={{ fontSize: '13px', color: 'var(--color-text-secondary)' }}>{runResult.detail}</div>
          </div>
        </div>
      )}

      {/* History table */}
      <Card>
        <div style={{ fontSize: '12px', fontWeight: 600, textTransform: 'uppercase', letterSpacing: '0.04em', color: 'var(--color-text-muted)', marginBottom: 'var(--space-4)' }}>
          Drill history
        </div>

        {loading ? (
          <div style={{ color: 'var(--color-text-muted)', fontSize: '13px' }}>Loading…</div>
        ) : history.length === 0 ? (
          <div style={{ color: 'var(--color-text-muted)', fontSize: '13px', padding: 'var(--space-4) 0' }}>
            No drills run yet. Run your first drill to verify your backups are restorable.
          </div>
        ) : (
          <div>
            {/* Table header */}
            <div style={{ display: 'grid', gridTemplateColumns: '100px 1fr 80px 80px', gap: 'var(--space-3)', padding: 'var(--space-2) 0', borderBottom: '1px solid var(--color-border)', fontSize: '11px', fontWeight: 600, textTransform: 'uppercase', letterSpacing: '0.03em', color: 'var(--color-text-muted)' }}>
              <span>Date</span>
              <span>Asset</span>
              <span>Source</span>
              <span>Result</span>
            </div>
            {history.map((drill, i) => (
              <div
                key={i}
                style={{
                  display: 'grid', gridTemplateColumns: '100px 1fr 80px 80px',
                  gap: 'var(--space-3)', padding: 'var(--space-3) 0',
                  borderBottom: '1px solid var(--color-border-subtle)',
                  fontSize: '13px',
                  background: !drill.pass ? 'rgba(224,80,80,0.04)' : 'transparent',
                }}
              >
                <span style={{ color: 'var(--color-text-muted)', fontSize: '12px' }}>
                  {new Date(drill.date).toLocaleDateString()}
                </span>
                <span style={{ overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap', fontFamily: 'var(--font-mono)', fontSize: '12px' }}>
                  {drill.asset}
                </span>
                <span style={{ fontSize: '12px', color: 'var(--color-text-muted)', textTransform: 'capitalize' }}>
                  {drill.source}
                </span>
                <span style={{ display: 'flex', alignItems: 'center', gap: 'var(--space-2)' }}>
                  {drill.pass
                    ? <><CheckCircleIcon size={13} color="var(--color-status-healthy)" /><span style={{ fontSize: '12px', color: 'var(--color-status-healthy)' }}>Pass</span></>
                    : <><XCircleIcon size={13} color="var(--color-status-critical)" /><span style={{ fontSize: '12px', color: 'var(--color-status-critical)', fontWeight: 700 }}>FAIL</span></>
                  }
                </span>
              </div>
            ))}
          </div>
        )}
      </Card>

      {/* What is a drill? */}
      <div style={{ marginTop: 'var(--space-5)', padding: 'var(--space-4)', background: 'var(--color-bg-elevated)', borderRadius: 'var(--radius-md)', border: '1px solid var(--color-border-subtle)' }}>
        <div style={{ fontSize: '12px', fontWeight: 600, color: 'var(--color-text-muted)', marginBottom: 'var(--space-2)', textTransform: 'uppercase', letterSpacing: '0.04em' }}>What is a restore drill?</div>
        <div style={{ fontSize: '13px', color: 'var(--color-text-secondary)', lineHeight: 1.6 }}>
          A restore drill picks a random backed-up file, restores it to a temporary location, and verifies its integrity.
          A passing drill means your backup is actually restorable — not just that files were written.
          Run drills regularly to catch corruption or key problems before you actually need to recover.
        </div>
      </div>
    </div>
  )
}
