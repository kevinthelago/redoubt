import { useEffect, useState } from 'react'
import Button from '../components/Button.jsx'
import Card from '../components/Card.jsx'
import ConfirmDialog from '../components/ConfirmDialog.jsx'
import { FolderIcon, DatabaseIcon, KeyIcon, FileIcon, PlusIcon, TrashIcon, RefreshIcon } from '../icons/index.jsx'

const CATEGORIES = [
  { id: 'source',   label: 'Source Code', Icon: FolderIcon   },
  { id: 'secrets',  label: 'Secrets',     Icon: KeyIcon      },
  { id: 'database', label: 'Database',    Icon: DatabaseIcon },
  { id: 'config',   label: 'Config',      Icon: FileIcon     },
]

const DB_PRESETS = [
  { label: 'PostgreSQL', cmd: 'pg_dump -Fc {database}' },
  { label: 'MySQL',      cmd: 'mysqldump --single-transaction {database}' },
  { label: 'SQLite',     cmd: 'sqlite3 {path} .dump' },
  { label: 'Custom',     cmd: '' },
]

export default function Assets() {
  const [assets, setAssets] = useState([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState(null)
  const [addMode, setAddMode] = useState(false)
  const [confirmRemove, setConfirmRemove] = useState(null) // path to remove

  // Add form state
  const [newPath, setNewPath] = useState('')
  const [newCategory, setNewCategory] = useState('source')
  const [newDumpCmd, setNewDumpCmd] = useState('')
  const [dbPreset, setDbPreset] = useState(DB_PRESETS[0])
  const [saving, setSaving] = useState(false)

  const load = async () => {
    setLoading(true)
    setError(null)
    try {
      const { ListAssets } = await import('../../wailsjs/go/main/App.js')
      setAssets(await ListAssets())
    } catch (e) {
      setError(e?.message ?? 'Failed to load assets')
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => { load() }, [])

  const handleAdd = async () => {
    if (!newPath) return
    setSaving(true)
    try {
      const { AddAsset } = await import('../../wailsjs/go/main/App.js')
      const dumpCmd = newCategory === 'database' ? (dbPreset.label === 'Custom' ? newDumpCmd : dbPreset.cmd) : ''
      await AddAsset(newPath, newCategory, dumpCmd)
      setAddMode(false)
      setNewPath('')
      setNewCategory('source')
      setNewDumpCmd('')
      await load()
    } catch (e) {
      setError(e?.message ?? 'Failed to add asset')
    } finally {
      setSaving(false)
    }
  }

  const handleRemove = async () => {
    if (!confirmRemove) return
    try {
      const { RemoveAsset } = await import('../../wailsjs/go/main/App.js')
      await RemoveAsset(confirmRemove)
      setConfirmRemove(null)
      await load()
    } catch (e) {
      setError(e?.message ?? 'Failed to remove asset')
    }
  }

  // Group assets by category.
  const byCategory = CATEGORIES.reduce((acc, cat) => {
    acc[cat.id] = assets.filter(a => a.category === cat.id)
    return acc
  }, {})

  return (
    <div style={{ maxWidth: 760 }}>
      <div style={{ display: 'flex', alignItems: 'center', gap: 'var(--space-4)', marginBottom: 'var(--space-6)' }}>
        <FolderIcon size={20} />
        <h1 style={{ fontSize: '20px', fontWeight: 700, flex: 1 }}>Assets</h1>
        <Button variant="ghost" onClick={load} disabled={loading} icon={RefreshIcon} size="sm">Refresh</Button>
        <Button variant="primary" onClick={() => setAddMode(true)} icon={PlusIcon} size="sm">Add asset</Button>
      </div>

      {error && (
        <div style={{ padding: 'var(--space-4)', background: 'rgba(224,80,80,0.1)', borderRadius: 'var(--radius-md)', color: 'var(--color-status-critical)', marginBottom: 'var(--space-4)' }}>
          {error}
        </div>
      )}

      {addMode && (
        <Card style={{ marginBottom: 'var(--space-5)' }}>
          <h2 style={{ fontSize: '14px', fontWeight: 600, marginBottom: 'var(--space-4)' }}>Add asset</h2>
          <div style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-4)' }}>
            <div>
              <label style={labelStyle}>Category</label>
              <div style={{ display: 'flex', gap: 'var(--space-2)' }}>
                {CATEGORIES.map(cat => (
                  <button
                    key={cat.id}
                    onClick={() => setNewCategory(cat.id)}
                    style={{
                      display: 'flex', alignItems: 'center', gap: 'var(--space-2)',
                      padding: '6px 12px', borderRadius: 'var(--radius-sm)',
                      border: `1px solid ${newCategory === cat.id ? 'var(--color-accent)' : 'var(--color-border)'}`,
                      background: newCategory === cat.id ? 'rgba(79,128,255,0.12)' : 'var(--color-bg-elevated)',
                      color: newCategory === cat.id ? 'var(--color-accent)' : 'var(--color-text-secondary)',
                      cursor: 'pointer', fontSize: '12px', fontFamily: 'var(--font-ui)',
                    }}
                  >
                    <cat.Icon size={13} />
                    {cat.label}
                  </button>
                ))}
              </div>
            </div>

            <div>
              <label style={labelStyle}>Path</label>
              <input
                type="text"
                value={newPath}
                onChange={(e) => setNewPath(e.target.value)}
                placeholder="/home/user/projects/myapp"
                style={inputStyle()}
              />
            </div>

            {newCategory === 'database' && (
              <div>
                <label style={labelStyle}>Database preset</label>
                <select
                  value={dbPreset.label}
                  onChange={(e) => setDbPreset(DB_PRESETS.find(p => p.label === e.target.value) ?? DB_PRESETS[0])}
                  style={{ ...inputStyle(), appearance: 'none' }}
                >
                  {DB_PRESETS.map(p => <option key={p.label} value={p.label}>{p.label}</option>)}
                </select>
                <div style={{ marginTop: 'var(--space-3)' }}>
                  <label style={labelStyle}>Dump command</label>
                  <input
                    type="text"
                    value={dbPreset.label === 'Custom' ? newDumpCmd : dbPreset.cmd}
                    onChange={(e) => { if (dbPreset.label === 'Custom') setNewDumpCmd(e.target.value) }}
                    readOnly={dbPreset.label !== 'Custom'}
                    style={{ ...inputStyle(), fontFamily: 'var(--font-mono)', fontSize: '12px', opacity: dbPreset.label !== 'Custom' ? 0.7 : 1 }}
                  />
                </div>
              </div>
            )}
          </div>

          <div style={{ display: 'flex', justifyContent: 'flex-end', gap: 'var(--space-3)', marginTop: 'var(--space-5)' }}>
            <Button variant="ghost" onClick={() => setAddMode(false)}>Cancel</Button>
            <Button variant="primary" disabled={!newPath || saving} onClick={handleAdd}>
              {saving ? 'Adding…' : 'Add asset'}
            </Button>
          </div>
        </Card>
      )}

      {loading ? (
        <div style={{ padding: 'var(--space-8)', color: 'var(--color-text-muted)' }}>Loading assets…</div>
      ) : (
        <div style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-6)' }}>
          {CATEGORIES.map(cat => {
            const catAssets = byCategory[cat.id] ?? []
            if (catAssets.length === 0) return null
            return (
              <div key={cat.id}>
                <div style={{ display: 'flex', alignItems: 'center', gap: 'var(--space-2)', marginBottom: 'var(--space-3)' }}>
                  <cat.Icon size={14} color="var(--color-text-muted)" />
                  <span style={{ fontSize: '12px', fontWeight: 600, textTransform: 'uppercase', letterSpacing: '0.04em', color: 'var(--color-text-muted)' }}>
                    {cat.label}
                  </span>
                  <span style={{ fontSize: '12px', color: 'var(--color-text-muted)' }}>({catAssets.length})</span>
                </div>
                <div style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-2)' }}>
                  {catAssets.map(asset => (
                    <Card key={asset.path}>
                      <div style={{ display: 'flex', alignItems: 'center', gap: 'var(--space-3)' }}>
                        <cat.Icon size={14} color="var(--color-text-muted)" />
                        <div style={{ flex: 1, minWidth: 0 }}>
                          <div style={{ fontSize: '13px', fontFamily: 'var(--font-mono)', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                            {asset.path}
                          </div>
                          {asset.dumpCommand && (
                            <div style={{ fontSize: '11px', color: 'var(--color-text-muted)', marginTop: 2, fontFamily: 'var(--font-mono)' }}>
                              {asset.dumpCommand}
                            </div>
                          )}
                          <div style={{ fontSize: '11px', color: 'var(--color-text-muted)', marginTop: 2 }}>
                            Added {asset.addedAt ? new Date(asset.addedAt).toLocaleDateString() : 'unknown'}
                          </div>
                        </div>
                        <Button
                          size="sm"
                          variant="ghost"
                          icon={TrashIcon}
                          onClick={() => setConfirmRemove(asset.path)}
                          aria-label={`Remove ${asset.path}`}
                        />
                      </div>
                    </Card>
                  ))}
                </div>
              </div>
            )
          })}

          {assets.length === 0 && (
            <div style={{ padding: 'var(--space-8)', textAlign: 'center', color: 'var(--color-text-muted)' }}>
              No assets tracked yet. Add a path to start protecting your data.
            </div>
          )}
        </div>
      )}

      {confirmRemove && (
        <ConfirmDialog
          title="Remove asset"
          message={`Remove "${confirmRemove}" from tracking? This does not delete existing snapshots.`}
          confirmLabel="Remove"
          confirmVariant="destructive"
          onConfirm={handleRemove}
          onCancel={() => setConfirmRemove(null)}
        />
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

function inputStyle() {
  return {
    width: '100%',
    padding: '8px 12px',
    fontFamily: 'var(--font-ui)',
    fontSize: '13px',
    background: 'var(--color-bg-primary)',
    border: '1px solid var(--color-border)',
    borderRadius: 'var(--radius-sm)',
    color: 'var(--color-text-primary)',
    outline: 'none',
  }
}
