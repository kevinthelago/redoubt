import { useEffect, useRef, useState } from 'react'
import Button from './Button.jsx'
import { XIcon } from '../icons/index.jsx'

/**
 * Modal confirmation dialog for destructive actions.
 * When `requireTyped` is provided, the user must type that string to confirm.
 *
 * Focus is trapped within the modal while open.
 */
export default function ConfirmDialog({
  title,
  message,
  confirmLabel = 'Confirm',
  cancelLabel = 'Cancel',
  confirmVariant = 'destructive',
  requireTyped,
  onConfirm,
  onCancel,
}) {
  const [typed, setTyped] = useState('')
  const cancelRef = useRef(null)
  const dialogRef = useRef(null)

  // Auto-focus cancel (safe default for destructive dialogs).
  useEffect(() => {
    cancelRef.current?.focus()
  }, [])

  // Trap focus within the dialog.
  useEffect(() => {
    const el = dialogRef.current
    if (!el) return
    const focusable = el.querySelectorAll(
      'button, input, [tabindex]:not([tabindex="-1"])'
    )
    const first = focusable[0]
    const last = focusable[focusable.length - 1]
    const handler = (e) => {
      if (e.key !== 'Tab') return
      if (e.shiftKey) {
        if (document.activeElement === first) { e.preventDefault(); last?.focus() }
      } else {
        if (document.activeElement === last) { e.preventDefault(); first?.focus() }
      }
    }
    el.addEventListener('keydown', handler)
    return () => el.removeEventListener('keydown', handler)
  }, [])

  // Close on Escape.
  useEffect(() => {
    const handler = (e) => { if (e.key === 'Escape') onCancel() }
    window.addEventListener('keydown', handler)
    return () => window.removeEventListener('keydown', handler)
  }, [onCancel])

  const canConfirm = requireTyped ? typed === requireTyped : true

  return (
    <div
      role="dialog"
      aria-modal="true"
      aria-labelledby="confirm-title"
      style={{
        position: 'fixed', inset: 0, zIndex: 1000,
        display: 'flex', alignItems: 'center', justifyContent: 'center',
        background: 'rgba(0,0,0,0.6)',
        backdropFilter: 'blur(2px)',
      }}
      onClick={(e) => { if (e.target === e.currentTarget) onCancel() }}
    >
      <div
        ref={dialogRef}
        style={{
          background: 'var(--color-bg-elevated)',
          border: '1px solid var(--color-border)',
          borderRadius: 'var(--radius-lg)',
          padding: 'var(--space-6)',
          width: 'min(480px, 90vw)',
          maxHeight: '90vh',
          overflow: 'auto',
          boxShadow: '0 24px 48px rgba(0,0,0,0.4)',
        }}
      >
        {/* Header */}
        <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start', marginBottom: 'var(--space-4)' }}>
          <h2 id="confirm-title" style={{ fontSize: '16px', fontWeight: 600 }}>{title}</h2>
          <button
            onClick={onCancel}
            style={{ background: 'none', border: 'none', color: 'var(--color-text-muted)', cursor: 'pointer', padding: 'var(--space-1)' }}
            aria-label="Close"
          >
            <XIcon size={16} />
          </button>
        </div>

        {/* Message */}
        <p style={{ color: 'var(--color-text-secondary)', marginBottom: requireTyped ? 'var(--space-4)' : 'var(--space-6)', lineHeight: 1.6 }}>
          {message}
        </p>

        {/* Typed confirmation */}
        {requireTyped && (
          <div style={{ marginBottom: 'var(--space-6)' }}>
            <label style={{ display: 'block', fontSize: '12px', color: 'var(--color-text-muted)', marginBottom: 'var(--space-2)' }}>
              Type <strong style={{ color: 'var(--color-text-primary)', fontFamily: 'var(--font-mono)' }}>{requireTyped}</strong> to confirm
            </label>
            <input
              type="text"
              value={typed}
              onChange={(e) => setTyped(e.target.value)}
              placeholder={requireTyped}
              autoComplete="off"
              style={{
                width: '100%',
                padding: '8px 12px',
                fontFamily: 'var(--font-mono)',
                fontSize: '13px',
                background: 'var(--color-bg-primary)',
                border: `1px solid ${typed && typed !== requireTyped ? 'var(--color-status-critical)' : 'var(--color-border)'}`,
                borderRadius: 'var(--radius-sm)',
                color: 'var(--color-text-primary)',
                outline: 'none',
              }}
            />
          </div>
        )}

        {/* Actions */}
        <div style={{ display: 'flex', gap: 'var(--space-3)', justifyContent: 'flex-end' }}>
          <Button ref={cancelRef} variant="secondary" onClick={onCancel}>
            {cancelLabel}
          </Button>
          <Button
            variant={confirmVariant}
            disabled={!canConfirm}
            onClick={onConfirm}
          >
            {confirmLabel}
          </Button>
        </div>
      </div>
    </div>
  )
}
