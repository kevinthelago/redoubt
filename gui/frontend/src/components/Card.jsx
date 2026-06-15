/**
 * Content card with an optional grade-colored left-edge stripe.
 *
 * @param {string} grade  - "healthy" | "warning" | "critical" | "unknown" | undefined
 * @param {string} title  - optional card title
 * @param {*} children    - card body
 */
export default function Card({ grade, title, children, className = '', style = {}, onClick }) {
  const gradeColor = {
    healthy:  'var(--color-status-healthy)',
    warning:  'var(--color-status-warning)',
    critical: 'var(--color-status-critical)',
    unknown:  'var(--color-status-unknown)',
  }

  const borderLeft = grade
    ? `3px solid ${gradeColor[grade] ?? gradeColor.unknown}`
    : '3px solid var(--color-border)'

  return (
    <div
      className={`card ${className}`}
      onClick={onClick}
      role={onClick ? 'button' : undefined}
      tabIndex={onClick ? 0 : undefined}
      onKeyDown={onClick ? (e) => e.key === 'Enter' && onClick(e) : undefined}
      style={{
        background: 'var(--color-bg-secondary)',
        border: '1px solid var(--color-border)',
        borderLeft,
        borderRadius: 'var(--radius-md)',
        padding: 'var(--space-4)',
        cursor: onClick ? 'pointer' : 'default',
        transition: `background var(--transition-fast)`,
        ...style,
      }}
    >
      {title && (
        <div style={{
          fontSize: '12px',
          fontWeight: 600,
          letterSpacing: '0.04em',
          textTransform: 'uppercase',
          color: 'var(--color-text-muted)',
          marginBottom: 'var(--space-3)',
        }}>
          {title}
        </div>
      )}
      {children}
    </div>
  )
}
