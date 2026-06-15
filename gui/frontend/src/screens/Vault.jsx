import { useEffect, useState } from 'react'
import Button from '../components/Button.jsx'
import Card from '../components/Card.jsx'
import GradeIcon from '../components/GradeIcon.jsx'
import { ServerIcon, RefreshIcon, AlertTriangleIcon } from '../icons/index.jsx'

const PRIVATE_IP_RE = /^(10\.|172\.(1[6-9]|2\d|3[01])\.|192\.168\.|localhost|127\.)/

function isPublicIP(addr) {
  // Strip protocol if present.
  const host = addr.replace(/^https?:\/\//, '').split('/')[0].split(':')[0]
  return !PRIVATE_IP_RE.test(host)
}

export default function Vault() {
  const [vaultStatus, setVaultStatus] = useState(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState(null)
  const [setupMode, setSetupMode] = useState(false)
  const [addr, setAddr] = useState('')
  const [repoPath, setRepoPath] = useState('')
  const [addrWarning, setAddrWarning] = useState(null)
  const [saving, setSaving] = useState(false)

  const load = async () => {
    setLoading(true)
    setError(null)
    try {
      const { GetVaultStatus } = await import('../../wailsjs/go/main/App.js')
      setVaultStatus(await GetVaultStatus())
    } catch (e) {
      setError(e?.message ?? 'Failed to load vault status')
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => { load() }, [])

  const handleAddrChange = (val) => {
    setAddr(val)
    if (val && isPublicIP(val)) {
      setAddrWarning('This address appears to be a public IP. Redoubt is designed for LAN use only — exposing your vault to the internet is not recommended.')
    } else {
      setAddrWarning(null)
    }
  }

  const handleSetup = async () => {
    if (!addr || !repoPath) return
    setSaving(true)
    try {
      const { SetupVault } = await import('../../wailsjs/go/main/App.js')
      await SetupVault(addr, repoPath)
      setSetupMode(false)
      await load()
    } catch (e) {
      setError(e?.message ?? 'Setup failed')
    } finally {
      setSaving(false)
    }
  }

  if (loading) return <div style={{ padding: 'var(--space-8)', color: 'var(--color-text-muted)' }}>Loading vault status…</div>

  return (
    <div style={{ maxWidth: 760 }}>
      <div style={{ display: 'flex', alignItems: 'center', gap: 'var(--space-4)', marginBottom: 'var(--space-6)' }}>
        <ServerIcon size={20} />
        <h1 style={{ fontSize: '20px', fontWeight: 700, flex: 1 }}>Vault</h1>
        <Button variant="ghost" onClick={load} disabled={loading} icon={RefreshIcon} size="sm">Refresh</Button>
      </div>

      {error && (
        <div style={{ padding: 'var(--space-4)', background: 'rgba(224,80,80,0.1)', borderRadius: 'var(--radius-md)', color: 'var(--color-status-critical)', marginBottom: 'var(--space-4)' }}>
          {error}
        </div>
      )}

      {!setupMode && vaultStatus ? (
        <div style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-4)' }}>
          {/* Configured / not configured */}
          <Card grade={vaultStatus.configured ? (vaultStatus.reachable ? 'healthy' : 'critical') : 'unknown'}>
            <div style={{ display: 'flex', alignItems: 'flex-start', gap: 'var(--space-4)' }}>
              <GradeIcon
                grade={vaultStatus.configured ? (vaultStatus.reachable ? 'healthy' : 'critical') : 'unknown'}
                size={20}
              />
              <div style={{ flex: 1 }}>
                <div style={{ fontWeight: 500, marginBottom: 'var(--space-1)' }}>
                  {!vaultStatus.configured && 'Not configured'}
                  {vaultStatus.configured && vaultStatus.reachable && 'Online and reachable'}
                  {vaultStatus.configured && !vaultStatus.reachable && 'Unreachable'}
                </div>
                {vaultStatus.url && (
                  <div style={{ fontSize: '13px', color: 'var(--color-text-secondary)', fontFamily: 'var(--font-mono)' }}>
                    {vaultStatus.url}
                  </div>
                )}
                {vaultStatus.repository && (
                  <div style={{ fontSize: '12px', color: 'var(--color-text-muted)', marginTop: 'var(--space-1)' }}>
                    Repository: {vaultStatus.repository}
                  </div>
                )}
              </div>
              <Button size="sm" variant="secondary" onClick={() => setSetupMode(true)}>
                {vaultStatus.configured ? 'Reconfigure' : 'Set up vault'}
              </Button>
            </div>
          </Card>

          {vaultStatus.configured && !vaultStatus.reachable && (
            <Card grade="critical">
              <div style={{ display: 'flex', alignItems: 'center', gap: 'var(--space-3)' }}>
                <AlertTriangleIcon size={16} color="var(--color-status-critical)" />
                <div style={{ fontSize: '13px' }}>
                  Vault is unreachable. Check that your vault server is running and this machine is on the same network.
                </div>
              </div>
            </Card>
          )}
        </div>
      ) : (
        /* Setup form */
        <Card>
          <h2 style={{ fontSize: '16px', fontWeight: 600, marginBottom: 'var(--space-4)' }}>
            {vaultStatus?.configured ? 'Reconfigure vault' : 'Connect to vault'}
          </h2>
          <p style={{ fontSize: '13px', color: 'var(--color-text-secondary)', marginBottom: 'var(--space-5)' }}>
            Enter the address of your Redoubt vault server. It must be reachable on your local network.
          </p>

          <div style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-4)' }}>
            <div>
              <label style={labelStyle}>Vault address</label>
              <input
                type="text"
                value={addr}
                onChange={(e) => handleAddrChange(e.target.value)}
                placeholder="192.168.1.100:8080"
                style={inputStyle()}
              />
              {addrWarning && (
                <div style={{ display: 'flex', alignItems: 'flex-start', gap: 'var(--space-2)', marginTop: 'var(--space-2)', padding: 'var(--space-3)', background: 'rgba(232,160,48,0.1)', borderRadius: 'var(--radius-sm)', fontSize: '12px', color: 'var(--color-status-warning)' }}>
                  <AlertTriangleIcon size={13} color="var(--color-status-warning)" style={{ flexShrink: 0, marginTop: 1 }} />
                  {addrWarning}
                </div>
              )}
            </div>

            <div>
              <label style={labelStyle}>Repository path (on vault server)</label>
              <input
                type="text"
                value={repoPath}
                onChange={(e) => setRepoPath(e.target.value)}
                placeholder="/mnt/backup/redoubt-repo"
                style={inputStyle()}
              />
            </div>
          </div>

          <div style={{ display: 'flex', justifyContent: 'space-between', marginTop: 'var(--space-6)' }}>
            <Button variant="ghost" onClick={() => setSetupMode(false)}>Cancel</Button>
            <Button
              variant="primary"
              disabled={!addr || !repoPath || saving}
              onClick={handleSetup}
            >
              {saving ? 'Connecting…' : 'Connect vault'}
            </Button>
          </div>
        </Card>
      )}
    </div>
  )
}

const labelStyle = {
  display: 'block',
  fontSize: '12px',
  color: 'var(--color-text-muted)',
  marginBottom: 'var(--space-2)',
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
