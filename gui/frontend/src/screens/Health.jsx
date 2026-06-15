import { useEffect, useState } from 'react'
import Card from '../components/Card.jsx'
import Button from '../components/Button.jsx'
import GradeIcon from '../components/GradeIcon.jsx'
import { RefreshIcon } from '../icons/index.jsx'

export default function Health({ health, onNavigate }) {
  const [status, setStatus] = useState(health)
  const [loading, setLoading] = useState(!health)
  const [error, setError] = useState(null)

  const refresh = async () => {
    setLoading(true)
    setError(null)
    try {
      const { GetStatus } = await import('../../wailsjs/go/main/App.js')
      setStatus(await GetStatus())
    } catch (e) {
      setError(e?.message ?? 'Failed to load status')
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => { if (!health) refresh() }, [])
  useEffect(() => { setStatus(health) }, [health])

  const signals = status?.signals ?? {}
  const evaluatedAt = status?.evaluated_at

  return (
    <div style={{ maxWidth: 760 }}>
      <div style={{ display: 'flex', alignItems: 'center', gap: 'var(--space-4)', marginBottom: 'var(--space-6)' }}>
        <h1 style={{ fontSize: '20px', fontWeight: 700, flex: 1 }}>System Health</h1>
        <Button variant="ghost" onClick={refresh} disabled={loading} icon={RefreshIcon} size="sm">
          {loading ? 'Refreshing…' : 'Refresh'}
        </Button>
      </div>

      {error && (
        <div style={{ padding: 'var(--space-4)', background: 'rgba(224,80,80,0.1)', borderRadius: 'var(--radius-md)', color: 'var(--color-status-critical)', marginBottom: 'var(--space-4)' }}>
          {error} — <button onClick={refresh} style={{ color: 'inherit', background: 'none', border: 'none', cursor: 'pointer', textDecoration: 'underline' }}>retry</button>
        </div>
      )}

      {evaluatedAt && (
        <p style={{ fontSize: '12px', color: 'var(--color-text-muted)', marginBottom: 'var(--space-4)' }}>
          Evaluated at {new Date(evaluatedAt).toLocaleString()}
        </p>
      )}

      <div style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-3)' }}>
        {Object.entries(signals).map(([key, sig]) => (
          <Card key={key} grade={sig.grade}>
            <div style={{ display: 'flex', alignItems: 'center', gap: 'var(--space-4)' }}>
              <GradeIcon grade={sig.grade} size={18} />
              <div style={{ flex: 1 }}>
                <div style={{ fontWeight: 500, fontSize: '13px', textTransform: 'capitalize', marginBottom: 'var(--space-1)' }}>
                  {key.replace(/_/g, ' ')}
                </div>
                <div style={{ fontSize: '13px', color: 'var(--color-text-secondary)' }}>{sig.message}</div>
              </div>
              <GradeIcon grade={sig.grade} showLabel size={13} />
            </div>
          </Card>
        ))}

        {Object.keys(signals).length === 0 && !loading && (
          <div style={{ padding: 'var(--space-8)', textAlign: 'center', color: 'var(--color-text-muted)' }}>
            No health signals yet. Complete setup to see health data.
          </div>
        )}
      </div>
    </div>
  )
}
