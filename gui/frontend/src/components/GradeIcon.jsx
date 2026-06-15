import { CheckCircleIcon, AlertTriangleIcon, XCircleIcon, HelpCircleIcon } from '../icons/index.jsx'

/**
 * GradeIcon renders the icon + label for a health grade.
 * NEVER uses color alone — always icon + text per the design contract.
 *
 * @param {string} grade - "healthy" | "warning" | "critical" | "unknown"
 * @param {number} size  - icon size in px
 * @param {boolean} showLabel - whether to show the text label alongside the icon
 */
export default function GradeIcon({ grade = 'unknown', size = 16, showLabel = false, className = '' }) {
  const config = {
    healthy:  { Icon: CheckCircleIcon,    label: 'Healthy',  color: 'var(--color-status-healthy)'  },
    warning:  { Icon: AlertTriangleIcon,  label: 'Warning',  color: 'var(--color-status-warning)'  },
    critical: { Icon: XCircleIcon,        label: 'Critical', color: 'var(--color-status-critical)' },
    unknown:  { Icon: HelpCircleIcon,     label: 'Unknown',  color: 'var(--color-status-unknown)'  },
  }

  const { Icon, label, color } = config[grade] ?? config.unknown

  return (
    <span
      className={`grade-icon grade-icon--${grade} ${className}`}
      style={{ display: 'inline-flex', alignItems: 'center', gap: 'var(--space-1)', color }}
      title={label}
    >
      <Icon size={size} color={color} aria-hidden="true" />
      {showLabel && <span style={{ fontSize: '0.875em', fontWeight: 500 }}>{label}</span>}
    </span>
  )
}
