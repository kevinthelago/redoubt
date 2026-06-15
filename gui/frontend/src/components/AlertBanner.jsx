import { useState } from 'react'
import { AlertTriangleIcon, XCircleIcon, XIcon } from '../icons/index.jsx'

/**
 * Dismissible alert strip shown at the top of the content area.
 * Only warning and critical alerts are shown.
 *
 * @param {Array} alerts - [{grade, message, action}]
 */
export default function AlertBanner({ alerts = [] }) {
  const [dismissed, setDismissed] = useState(new Set())

  const visible = alerts.filter(
    (a, i) => (a.grade === 'warning' || a.grade === 'critical') && !dismissed.has(i)
  )

  if (visible.length === 0) return null

  return (
    <div role="alert" aria-live="polite" style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-1)' }}>
      {visible.map((alert, i) => {
        const isCritical = alert.grade === 'critical'
        const Icon = isCritical ? XCircleIcon : AlertTriangleIcon
        const color = isCritical ? 'var(--color-status-critical)' : 'var(--color-status-warning)'
        const bg = isCritical ? 'rgba(224,80,80,0.1)' : 'rgba(232,160,48,0.1)'

        return (
          <div
            key={i}
            style={{
              display: 'flex',
              alignItems: 'flex-start',
              gap: 'var(--space-3)',
              padding: 'var(--space-3) var(--space-4)',
              background: bg,
              borderBottom: `1px solid ${color}22`,
            }}
          >
            <Icon size={16} color={color} style={{ flexShrink: 0, marginTop: 1 }} />
            <div style={{ flex: 1 }}>
              <span style={{ color: 'var(--color-text-primary)', fontSize: '13px' }}>{alert.message}</span>
              {alert.action && (
                <span style={{ marginLeft: 'var(--space-3)', color, fontSize: '12px', fontWeight: 500 }}>
                  {alert.action}
                </span>
              )}
            </div>
            <button
              onClick={() => setDismissed(d => new Set([...d, i]))}
              aria-label="Dismiss alert"
              style={{ background: 'none', border: 'none', cursor: 'pointer', color: 'var(--color-text-muted)', flexShrink: 0 }}
            >
              <XIcon size={14} />
            </button>
          </div>
        )
      })}
    </div>
  )
}
