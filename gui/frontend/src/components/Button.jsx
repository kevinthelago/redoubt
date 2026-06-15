/**
 * Base button component.
 * Variants: primary | secondary | destructive | ghost
 */
export default function Button({
  children,
  variant = 'secondary',
  size = 'md',
  disabled = false,
  onClick,
  type = 'button',
  className = '',
  icon: Icon,
  ...rest
}) {
  const styles = {
    display: 'inline-flex',
    alignItems: 'center',
    justifyContent: 'center',
    gap: 'var(--space-2)',
    fontFamily: 'var(--font-ui)',
    fontWeight: 500,
    borderRadius: 'var(--radius-sm)',
    cursor: disabled ? 'not-allowed' : 'pointer',
    opacity: disabled ? 0.5 : 1,
    border: '1px solid transparent',
    transition: `background var(--transition-fast), color var(--transition-fast), border-color var(--transition-fast)`,
    outline: 'none',
    textDecoration: 'none',
    whiteSpace: 'nowrap',
  }

  const sizeStyles = {
    sm: { padding: '4px 10px', fontSize: '12px' },
    md: { padding: '7px 14px', fontSize: '13px' },
    lg: { padding: '10px 20px', fontSize: '14px' },
  }

  const variantStyles = {
    primary: {
      background: 'var(--color-accent)',
      color: '#fff',
      borderColor: 'var(--color-accent)',
    },
    secondary: {
      background: 'var(--color-bg-elevated)',
      color: 'var(--color-text-primary)',
      borderColor: 'var(--color-border)',
    },
    destructive: {
      background: 'var(--color-destructive)',
      color: '#fff',
      borderColor: 'var(--color-destructive)',
    },
    ghost: {
      background: 'transparent',
      color: 'var(--color-text-secondary)',
      borderColor: 'transparent',
    },
  }

  return (
    <button
      type={type}
      disabled={disabled}
      onClick={onClick}
      className={`btn btn--${variant} btn--${size} ${className}`}
      style={{ ...styles, ...sizeStyles[size], ...variantStyles[variant] }}
      {...rest}
    >
      {Icon && <Icon size={14} />}
      {children}
    </button>
  )
}
