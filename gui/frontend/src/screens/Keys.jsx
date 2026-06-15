import { useEffect, useRef, useState } from 'react'
import Button from '../components/Button.jsx'
import Card from '../components/Card.jsx'
import GradeIcon from '../components/GradeIcon.jsx'
import { KeyIcon, CheckCircleIcon, AlertTriangleIcon, PrinterIcon, RefreshIcon } from '../icons/index.jsx'

const STEPS = ['Passphrase', 'Generate Key', 'Split Shares', 'Verify Escrow', 'Done']

export default function Keys() {
  const [keyStatus, setKeyStatus] = useState(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState(null)
  const [wizardActive, setWizardActive] = useState(false)
  const [wizardStep, setWizardStep] = useState(0)

  const load = async () => {
    setLoading(true)
    setError(null)
    try {
      const { GetKeyStatus } = await import('../../wailsjs/go/main/App.js')
      setKeyStatus(await GetKeyStatus())
    } catch (e) {
      setError(e?.message ?? 'Failed to load key status')
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => { load() }, [])

  if (loading) return <LoadingState />
  if (error) return <ErrorState message={error} onRetry={load} />

  if (!keyStatus?.exists || wizardActive) {
    return (
      <KeyWizard
        step={wizardStep}
        onStep={setWizardStep}
        onDone={() => { setWizardActive(false); setWizardStep(0); load() }}
        onCancel={() => { setWizardActive(false); setWizardStep(0) }}
      />
    )
  }

  return (
    <div style={{ maxWidth: 760 }}>
      <div style={{ display: 'flex', alignItems: 'center', gap: 'var(--space-4)', marginBottom: 'var(--space-6)' }}>
        <KeyIcon size={20} />
        <h1 style={{ fontSize: '20px', fontWeight: 700, flex: 1 }}>Encryption Key</h1>
        <Button variant="ghost" onClick={load} disabled={loading} icon={RefreshIcon} size="sm">Refresh</Button>
      </div>

      {/* Key state cards */}
      <div style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-4)' }}>
        <Card grade={keyStatus.exists ? 'healthy' : 'critical'}>
          <div style={{ display: 'flex', alignItems: 'center', gap: 'var(--space-4)' }}>
            <GradeIcon grade={keyStatus.exists ? 'healthy' : 'critical'} size={20} />
            <div>
              <div style={{ fontWeight: 500 }}>Key {keyStatus.exists ? 'exists' : 'not found'}</div>
              <div style={{ fontSize: '12px', color: 'var(--color-text-secondary)' }}>
                {keyStatus.exists ? 'Encryption key is present on this machine.' : 'No encryption key found. Create one to begin.'}
              </div>
            </div>
          </div>
        </Card>

        <Card grade={keyStatus.escrowVerified ? 'healthy' : 'warning'}>
          <div style={{ display: 'flex', alignItems: 'center', gap: 'var(--space-4)' }}>
            <GradeIcon grade={keyStatus.escrowVerified ? 'healthy' : 'warning'} size={20} />
            <div style={{ flex: 1 }}>
              <div style={{ fontWeight: 500 }}>Escrow {keyStatus.escrowVerified ? 'verified' : 'not verified'}</div>
              <div style={{ fontSize: '12px', color: 'var(--color-text-secondary)' }}>
                {keyStatus.escrowVerified
                  ? `${keyStatus.shareCount} shares stored. Last verified: ${keyStatus.lastVerified ?? 'never'}.`
                  : 'Key escrow has not been verified. Run the wizard to verify.'}
              </div>
            </div>
            {!keyStatus.escrowVerified && (
              <Button size="sm" variant="primary" onClick={() => setWizardActive(true)}>Verify escrow</Button>
            )}
          </div>
        </Card>

        {keyStatus.escrowVerified && (
          <Card grade="healthy">
            <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
              <div>
                <div style={{ fontWeight: 500, marginBottom: 'var(--space-1)' }}>Recovery sheet</div>
                <div style={{ fontSize: '12px', color: 'var(--color-text-secondary)' }}>
                  Print the recovery sheet and store it securely.
                </div>
              </div>
              <Button size="sm" variant="secondary" icon={PrinterIcon} onClick={() => window.print()}>
                Print recovery sheet
              </Button>
            </div>
          </Card>
        )}

        {/* Management actions */}
        <div style={{ display: 'flex', gap: 'var(--space-3)', marginTop: 'var(--space-2)' }}>
          <Button variant="secondary" onClick={() => { setWizardActive(true); setWizardStep(0) }}>
            Re-run wizard
          </Button>
          <Button variant="ghost" onClick={() => setWizardActive(true)}>
            Rotate key
          </Button>
        </div>
      </div>

      {/* Print-only recovery sheet */}
      <PrintableRecoverySheet shares={keyStatus.shareCount} lastVerified={keyStatus.lastVerified} />
    </div>
  )
}

function KeyWizard({ step, onStep, onDone, onCancel }) {
  const [passphrase, setPassphrase] = useState('')
  const [confirm, setConfirm] = useState('')
  const [strength, setStrength] = useState(0)

  const passphraseStrength = (p) => {
    let score = 0
    if (p.length >= 12) score++
    if (p.length >= 20) score++
    if (/[A-Z]/.test(p)) score++
    if (/[0-9]/.test(p)) score++
    if (/[^A-Za-z0-9]/.test(p)) score++
    return score
  }

  useEffect(() => { setStrength(passphraseStrength(passphrase)) }, [passphrase])

  const strengthLabel = ['Very weak', 'Weak', 'Fair', 'Good', 'Strong', 'Very strong'][strength] ?? 'Unknown'
  const strengthColor = ['var(--color-status-critical)', 'var(--color-status-critical)',
    'var(--color-status-warning)', 'var(--color-status-warning)',
    'var(--color-status-healthy)', 'var(--color-status-healthy)'][strength]

  return (
    <div style={{ maxWidth: 600 }}>
      {/* Step indicator */}
      <div style={{ display: 'flex', gap: 'var(--space-2)', marginBottom: 'var(--space-6)', alignItems: 'center' }}>
        {STEPS.map((s, i) => (
          <div key={s} style={{ display: 'flex', alignItems: 'center', gap: 'var(--space-2)' }}>
            <div style={{
              width: 24, height: 24, borderRadius: '50%',
              background: i < step ? 'var(--color-status-healthy)' : i === step ? 'var(--color-accent)' : 'var(--color-bg-elevated)',
              border: `1px solid ${i === step ? 'var(--color-accent)' : 'var(--color-border)'}`,
              display: 'flex', alignItems: 'center', justifyContent: 'center',
              fontSize: '11px', fontWeight: 600,
              color: i <= step ? '#fff' : 'var(--color-text-muted)',
            }}>
              {i < step ? '✓' : i + 1}
            </div>
            <span style={{ fontSize: '12px', color: i === step ? 'var(--color-text-primary)' : 'var(--color-text-muted)', fontWeight: i === step ? 500 : 400 }}>
              {s}
            </span>
            {i < STEPS.length - 1 && <span style={{ color: 'var(--color-border)', margin: '0 var(--space-1)' }}>—</span>}
          </div>
        ))}
      </div>

      {/* Step 0: Passphrase */}
      {step === 0 && (
        <Card>
          <h2 style={{ fontSize: '16px', fontWeight: 600, marginBottom: 'var(--space-4)' }}>Create a passphrase</h2>
          <p style={{ fontSize: '13px', color: 'var(--color-text-secondary)', marginBottom: 'var(--space-4)' }}>
            This passphrase protects your encryption key. Use a long, memorable phrase — not a password you reuse elsewhere.
          </p>
          <div style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-4)' }}>
            <div>
              <label style={{ display: 'block', fontSize: '12px', color: 'var(--color-text-muted)', marginBottom: 'var(--space-2)' }}>Passphrase</label>
              <input
                type="password"
                value={passphrase}
                onChange={(e) => setPassphrase(e.target.value)}
                autoComplete="new-password"
                style={inputStyle()}
              />
              {passphrase && (
                <div style={{ display: 'flex', alignItems: 'center', gap: 'var(--space-2)', marginTop: 'var(--space-2)' }}>
                  <div style={{ flex: 1, height: 4, background: 'var(--color-bg-elevated)', borderRadius: 2 }}>
                    <div style={{ width: `${(strength / 5) * 100}%`, height: '100%', background: strengthColor, borderRadius: 2, transition: 'width var(--transition-fast)' }} />
                  </div>
                  <span style={{ fontSize: '11px', color: strengthColor, fontWeight: 500 }}>{strengthLabel}</span>
                </div>
              )}
            </div>
            <div>
              <label style={{ display: 'block', fontSize: '12px', color: 'var(--color-text-muted)', marginBottom: 'var(--space-2)' }}>Confirm passphrase</label>
              <input
                type="password"
                value={confirm}
                onChange={(e) => setConfirm(e.target.value)}
                autoComplete="new-password"
                style={inputStyle(confirm && confirm !== passphrase)}
              />
              {confirm && confirm !== passphrase && (
                <div style={{ fontSize: '12px', color: 'var(--color-status-critical)', marginTop: 'var(--space-1)' }}>Passphrases do not match</div>
              )}
            </div>
          </div>
          <div style={{ display: 'flex', justifyContent: 'flex-end', gap: 'var(--space-3)', marginTop: 'var(--space-5)' }}>
            <Button variant="ghost" onClick={onCancel}>Cancel</Button>
            <Button variant="primary" disabled={!passphrase || passphrase !== confirm || strength < 2} onClick={() => onStep(1)}>Next</Button>
          </div>
        </Card>
      )}

      {/* Step 1: Generate key */}
      {step === 1 && (
        <Card>
          <h2 style={{ fontSize: '16px', fontWeight: 600, marginBottom: 'var(--space-4)' }}>Generate encryption key</h2>
          <p style={{ fontSize: '13px', color: 'var(--color-text-secondary)', marginBottom: 'var(--space-4)' }}>
            Redoubt will generate a strong random key and encrypt it with your passphrase. The key never leaves this machine unencrypted.
          </p>
          <div style={{ display: 'flex', justifyContent: 'space-between', marginTop: 'var(--space-5)' }}>
            <Button variant="ghost" onClick={() => onStep(0)}>Back</Button>
            <Button variant="primary" onClick={() => onStep(2)}>Generate key</Button>
          </div>
        </Card>
      )}

      {/* Step 2: Shamir split */}
      {step === 2 && (
        <Card>
          <h2 style={{ fontSize: '16px', fontWeight: 600, marginBottom: 'var(--space-4)' }}>Split into recovery shares</h2>
          <p style={{ fontSize: '13px', color: 'var(--color-text-secondary)', marginBottom: 'var(--space-4)' }}>
            Your key is split into 3 shares using Shamir's Secret Sharing. Any 2 shares can reconstruct the key.
            Store each share separately — USB drive, printed copy, trusted person.
          </p>
          <div style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-3)', marginBottom: 'var(--space-5)' }}>
            {['Share 1 — USB drive', 'Share 2 — Printed copy', 'Share 3 — Trusted contact'].map((s, i) => (
              <div key={i} style={{
                padding: 'var(--space-3) var(--space-4)',
                background: 'var(--color-bg-elevated)',
                borderRadius: 'var(--radius-md)',
                border: '1px solid var(--color-border)',
                fontSize: '13px',
                display: 'flex', alignItems: 'center', gap: 'var(--space-3)',
              }}>
                <span style={{ fontFamily: 'var(--font-mono)', fontSize: '11px', color: 'var(--color-text-muted)' }}>{i + 1}/3</span>
                {s}
              </div>
            ))}
          </div>
          <div style={{ display: 'flex', justifyContent: 'space-between' }}>
            <Button variant="ghost" onClick={() => onStep(1)}>Back</Button>
            <Button variant="primary" onClick={() => onStep(3)}>Shares distributed</Button>
          </div>
        </Card>
      )}

      {/* Step 3: Verify escrow */}
      {step === 3 && (
        <Card>
          <h2 style={{ fontSize: '16px', fontWeight: 600, marginBottom: 'var(--space-4)' }}>Verify escrow</h2>
          <p style={{ fontSize: '13px', color: 'var(--color-text-secondary)', marginBottom: 'var(--space-4)' }}>
            Reconstruct the key from 2 of your 3 shares to confirm you can recover your data if you lose this machine.
            This step is significant — it proves your backup plan actually works.
          </p>
          <div style={{ padding: 'var(--space-4)', background: 'rgba(79,128,255,0.08)', border: '1px solid rgba(79,128,255,0.2)', borderRadius: 'var(--radius-md)', marginBottom: 'var(--space-5)', fontSize: '13px' }}>
            <strong>2 of 3 shares</strong> are enough to recover your key.
          </div>
          <div style={{ display: 'flex', justifyContent: 'space-between' }}>
            <Button variant="ghost" onClick={() => onStep(2)}>Back</Button>
            <Button variant="primary" onClick={() => onStep(4)}>Escrow verified</Button>
          </div>
        </Card>
      )}

      {/* Step 4: Done */}
      {step === 4 && (
        <Card grade="healthy">
          <div style={{ textAlign: 'center', padding: 'var(--space-6)' }}>
            <div style={{ fontSize: 48, marginBottom: 'var(--space-4)' }}>
              <CheckCircleIcon size={48} color="var(--color-status-healthy)" />
            </div>
            <h2 style={{ fontSize: '20px', fontWeight: 700, marginBottom: 'var(--space-3)' }}>Key set up and verified</h2>
            <p style={{ fontSize: '13px', color: 'var(--color-text-secondary)', marginBottom: 'var(--space-6)', maxWidth: 400, margin: '0 auto var(--space-6)' }}>
              Your encryption key is secure and your escrow is verified. Your data will be encrypted with this key.
            </p>
            <Button variant="primary" onClick={onDone}>Done</Button>
          </div>
        </Card>
      )}
    </div>
  )
}

function PrintableRecoverySheet({ shares, lastVerified }) {
  return (
    <div className="print-only" style={{ marginTop: 40 }}>
      <h1>Redoubt Recovery Sheet</h1>
      <p>Generated: {new Date().toLocaleDateString()}</p>
      <p>Shares: {shares} (any 2 of {shares} can reconstruct the key)</p>
      <p>Last verified: {lastVerified ?? 'Never'}</p>
      <p style={{ marginTop: 20 }}>
        <strong>Instructions:</strong> In an emergency, use 2 of your stored shares to reconstruct the encryption key
        using the Redoubt CLI: <code>redoubt key recover</code>
      </p>
    </div>
  )
}

function LoadingState() {
  return <div style={{ padding: 'var(--space-8)', color: 'var(--color-text-muted)' }}>Loading key status…</div>
}

function ErrorState({ message, onRetry }) {
  return (
    <div style={{ padding: 'var(--space-6)' }}>
      <div style={{ color: 'var(--color-status-critical)', marginBottom: 'var(--space-4)' }}>{message}</div>
      <Button onClick={onRetry}>Retry</Button>
    </div>
  )
}

function inputStyle(hasError = false) {
  return {
    width: '100%',
    padding: '8px 12px',
    fontFamily: 'var(--font-ui)',
    fontSize: '13px',
    background: 'var(--color-bg-primary)',
    border: `1px solid ${hasError ? 'var(--color-status-critical)' : 'var(--color-border)'}`,
    borderRadius: 'var(--radius-sm)',
    color: 'var(--color-text-primary)',
    outline: 'none',
  }
}
