import { useEffect, useState } from 'react'
import Button from '../components/Button.jsx'
import Card from '../components/Card.jsx'
import { CameraIcon, RefreshIcon, FilterIcon, SearchIcon, LockIcon, FileIcon, FolderIcon, ChevronRightIcon, ChevronDownIcon } from '../icons/index.jsx'

export default function Snapshots() {
  const [snapshots, setSnapshots] = useState([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState(null)
  const [source, setSource] = useState('vault') // 'vault' | 'cold'
  const [filter, setFilter] = useState({ tags: [], hostname: '', after: '', before: '', source: 'vault' })
  const [showFilter, setShowFilter] = useState(false)
  const [selected, setSelected] = useState(null) // selected snapshot
  const [treeData, setTreeData] = useState([])
  const [treeLoading, setTreeLoading] = useState(false)
  const [findQuery, setFindQuery] = useState('')
  const [findResults, setFindResults] = useState(null)
  const [diffA, setDiffA] = useState('')
  const [diffB, setDiffB] = useState('')
  const [diffResults, setDiffResults] = useState(null)
  const [view, setView] = useState('list') // 'list' | 'tree' | 'find' | 'diff'

  const load = async () => {
    setLoading(true)
    setError(null)
    try {
      const { ListSnapshots } = await import('../../wailsjs/go/main/App.js')
      const snaps = await ListSnapshots({ ...filter, source })
      setSnapshots(snaps)
    } catch (e) {
      setError(e?.message ?? 'Failed to load snapshots')
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => { load() }, [source])

  const loadTree = async (id) => {
    setTreeLoading(true)
    try {
      const { GetSnapshotTree } = await import('../../wailsjs/go/main/App.js')
      setTreeData(await GetSnapshotTree(id, ''))
    } catch (e) {
      setError(e?.message ?? 'Failed to load tree')
    } finally {
      setTreeLoading(false)
    }
  }

  const handleSelectSnapshot = (snap) => {
    setSelected(snap)
    setView('tree')
    loadTree(snap.id)
  }

  const handleFind = async () => {
    if (!findQuery) return
    try {
      const { FindInSnapshots } = await import('../../wailsjs/go/main/App.js')
      setFindResults(await FindInSnapshots(findQuery))
    } catch (e) {
      setError(e?.message ?? 'Find failed')
    }
  }

  const handleDiff = async () => {
    if (!diffA || !diffB) return
    try {
      const { DiffSnapshots } = await import('../../wailsjs/go/main/App.js')
      setDiffResults(await DiffSnapshots(diffA, diffB))
    } catch (e) {
      setError(e?.message ?? 'Diff failed')
    }
  }

  const formatBytes = (b) => {
    if (!b) return '—'
    const units = ['B', 'KB', 'MB', 'GB', 'TB']
    let i = 0; let v = b
    while (v >= 1024 && i < units.length - 1) { v /= 1024; i++ }
    return `${v.toFixed(1)} ${units[i]}`
  }

  return (
    <div style={{ maxWidth: 960 }}>
      {/* Header */}
      <div style={{ display: 'flex', alignItems: 'center', gap: 'var(--space-4)', marginBottom: 'var(--space-5)' }}>
        <CameraIcon size={20} />
        <h1 style={{ fontSize: '20px', fontWeight: 700, flex: 1 }}>Snapshots</h1>
        <div style={{ display: 'flex', gap: 'var(--space-2)' }}>
          {['vault', 'cold'].map(s => (
            <button
              key={s}
              onClick={() => setSource(s)}
              style={{
                padding: '5px 14px', fontSize: '12px', borderRadius: 'var(--radius-sm)',
                border: `1px solid ${source === s ? 'var(--color-accent)' : 'var(--color-border)'}`,
                background: source === s ? 'rgba(79,128,255,0.12)' : 'transparent',
                color: source === s ? 'var(--color-accent)' : 'var(--color-text-secondary)',
                cursor: 'pointer', fontFamily: 'var(--font-ui)', fontWeight: source === s ? 500 : 400,
                textTransform: 'capitalize',
              }}
            >
              {s === 'vault' ? 'Vault' : 'Cold drive'}
            </button>
          ))}
        </div>
        <Button variant="ghost" onClick={() => setShowFilter(f => !f)} icon={FilterIcon} size="sm">Filter</Button>
        <Button variant="ghost" onClick={load} disabled={loading} icon={RefreshIcon} size="sm">Refresh</Button>
      </div>

      {/* View tabs */}
      <div style={{ display: 'flex', gap: 'var(--space-1)', marginBottom: 'var(--space-4)', borderBottom: '1px solid var(--color-border)', paddingBottom: 'var(--space-3)' }}>
        {[
          { id: 'list', label: 'Snapshot list' },
          { id: 'tree', label: 'File tree', disabled: !selected },
          { id: 'find', label: 'Find in snapshots' },
          { id: 'diff', label: 'Diff snapshots' },
        ].map(tab => (
          <button
            key={tab.id}
            onClick={() => !tab.disabled && setView(tab.id)}
            disabled={tab.disabled}
            style={{
              padding: '5px 12px', fontSize: '12px', borderRadius: 'var(--radius-sm)',
              border: 'none', background: view === tab.id ? 'var(--color-surface-active)' : 'transparent',
              color: tab.disabled ? 'var(--color-text-muted)' : view === tab.id ? 'var(--color-text-primary)' : 'var(--color-text-secondary)',
              cursor: tab.disabled ? 'not-allowed' : 'pointer', fontFamily: 'var(--font-ui)',
              fontWeight: view === tab.id ? 500 : 400,
            }}
          >
            {tab.label}
          </button>
        ))}
      </div>

      {error && (
        <div style={{ padding: 'var(--space-3)', background: 'rgba(224,80,80,0.1)', borderRadius: 'var(--radius-md)', color: 'var(--color-status-critical)', marginBottom: 'var(--space-4)', fontSize: '13px' }}>
          {error}
        </div>
      )}

      {/* Filter panel */}
      {showFilter && (
        <Card style={{ marginBottom: 'var(--space-4)' }}>
          <div style={{ display: 'flex', gap: 'var(--space-4)', flexWrap: 'wrap' }}>
            <div style={{ flex: 1, minWidth: 160 }}>
              <label style={labelStyle}>Hostname</label>
              <input type="text" value={filter.hostname} onChange={e => setFilter(f => ({ ...f, hostname: e.target.value }))} placeholder="All hosts" style={inputStyle()} />
            </div>
            <div style={{ flex: 1, minWidth: 140 }}>
              <label style={labelStyle}>After</label>
              <input type="date" value={filter.after} onChange={e => setFilter(f => ({ ...f, after: e.target.value }))} style={inputStyle()} />
            </div>
            <div style={{ flex: 1, minWidth: 140 }}>
              <label style={labelStyle}>Before</label>
              <input type="date" value={filter.before} onChange={e => setFilter(f => ({ ...f, before: e.target.value }))} style={inputStyle()} />
            </div>
          </div>
          <div style={{ display: 'flex', justifyContent: 'flex-end', gap: 'var(--space-3)', marginTop: 'var(--space-4)' }}>
            <Button size="sm" variant="ghost" onClick={() => setFilter({ tags: [], hostname: '', after: '', before: '', source })}>Clear</Button>
            <Button size="sm" variant="primary" onClick={load}>Apply</Button>
          </div>
        </Card>
      )}

      {/* Snapshot list */}
      {view === 'list' && (
        <div>
          {loading ? (
            <div style={{ padding: 'var(--space-8)', color: 'var(--color-text-muted)' }}>Loading snapshots…</div>
          ) : snapshots.length === 0 ? (
            <div style={{ padding: 'var(--space-8)', textAlign: 'center', color: 'var(--color-text-muted)' }}>
              No snapshots found. Run a backup to create one.
            </div>
          ) : (
            <div style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-2)' }}>
              {snapshots.map(snap => (
                <Card
                  key={snap.id}
                  onClick={() => handleSelectSnapshot(snap)}
                  style={{ cursor: 'pointer' }}
                >
                  <div style={{ display: 'flex', alignItems: 'center', gap: 'var(--space-4)' }}>
                    <div style={{ flex: 1, minWidth: 0 }}>
                      <div style={{ display: 'flex', alignItems: 'center', gap: 'var(--space-3)', marginBottom: 'var(--space-1)' }}>
                        <span style={{ fontFamily: 'var(--font-mono)', fontSize: '12px', color: 'var(--color-accent)' }}>{snap.shortId}</span>
                        <span style={{ fontSize: '13px', color: 'var(--color-text-primary)' }}>
                          {new Date(snap.time).toLocaleString()}
                        </span>
                        {snap.hostname && <span style={{ fontSize: '12px', color: 'var(--color-text-muted)' }}>{snap.hostname}</span>}
                      </div>
                      <div style={{ display: 'flex', gap: 'var(--space-3)', fontSize: '12px', color: 'var(--color-text-muted)' }}>
                        <span>{formatBytes(snap.bytesAdded)} added</span>
                        {snap.paths?.length > 0 && <span>{snap.paths.length} path{snap.paths.length !== 1 ? 's' : ''}</span>}
                        {snap.tags?.map(t => (
                          <span key={t} style={{ padding: '1px 6px', background: 'var(--color-bg-elevated)', borderRadius: 'var(--radius-sm)', border: '1px solid var(--color-border)' }}>{t}</span>
                        ))}
                      </div>
                    </div>
                    <ChevronRightIcon size={14} color="var(--color-text-muted)" />
                  </div>
                </Card>
              ))}
            </div>
          )}
        </div>
      )}

      {/* File tree */}
      {view === 'tree' && selected && (
        <div>
          <div style={{ marginBottom: 'var(--space-3)', fontSize: '13px', color: 'var(--color-text-secondary)' }}>
            Snapshot <span style={{ fontFamily: 'var(--font-mono)', color: 'var(--color-accent)' }}>{selected.shortId}</span>
            {' '}&mdash; {new Date(selected.time).toLocaleString()}
          </div>
          {treeLoading ? (
            <div style={{ padding: 'var(--space-6)', color: 'var(--color-text-muted)' }}>Loading file tree…</div>
          ) : (
            <TreeView nodes={treeData} />
          )}
        </div>
      )}

      {/* Find */}
      {view === 'find' && (
        <div>
          <div style={{ display: 'flex', gap: 'var(--space-3)', marginBottom: 'var(--space-4)' }}>
            <input
              type="text"
              value={findQuery}
              onChange={e => setFindQuery(e.target.value)}
              onKeyDown={e => e.key === 'Enter' && handleFind()}
              placeholder="Search for a file path across all snapshots…"
              style={{ ...inputStyle(), flex: 1 }}
            />
            <Button variant="primary" onClick={handleFind} icon={SearchIcon} disabled={!findQuery}>Search</Button>
          </div>
          {findResults && (
            <div style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-4)' }}>
              {findResults.length === 0 ? (
                <div style={{ color: 'var(--color-text-muted)', fontSize: '13px' }}>No matches found.</div>
              ) : findResults.map((r, i) => (
                <Card key={i}>
                  <div style={{ fontSize: '12px', color: 'var(--color-text-muted)', marginBottom: 'var(--space-2)' }}>
                    Snapshot <span style={{ fontFamily: 'var(--font-mono)', color: 'var(--color-accent)' }}>{r.snapshot.shortId}</span>
                    {' '}&mdash; {new Date(r.snapshot.time).toLocaleString()}
                  </div>
                  {r.matches.map(m => (
                    <div key={m.path} style={{ display: 'flex', alignItems: 'center', gap: 'var(--space-2)', padding: 'var(--space-2) 0', borderTop: '1px solid var(--color-border-subtle)', fontSize: '13px' }}>
                      {m.isSealed ? <LockIcon size={13} color="var(--color-text-muted)" /> : <FileIcon size={13} color="var(--color-text-muted)" />}
                      <span style={{ fontFamily: 'var(--font-mono)', flex: 1 }}>{m.path}</span>
                      {m.isSealed && <span style={{ fontSize: '11px', color: 'var(--color-text-muted)' }}>sealed</span>}
                    </div>
                  ))}
                </Card>
              ))}
            </div>
          )}
        </div>
      )}

      {/* Diff */}
      {view === 'diff' && (
        <div>
          <div style={{ display: 'flex', gap: 'var(--space-4)', marginBottom: 'var(--space-4)', alignItems: 'flex-end' }}>
            <div style={{ flex: 1 }}>
              <label style={labelStyle}>Snapshot A (earlier)</label>
              <select value={diffA} onChange={e => setDiffA(e.target.value)} style={{ ...inputStyle(), appearance: 'none' }}>
                <option value="">Select snapshot…</option>
                {snapshots.map(s => (
                  <option key={s.id} value={s.id}>{s.shortId} — {new Date(s.time).toLocaleString()}</option>
                ))}
              </select>
            </div>
            <div style={{ flex: 1 }}>
              <label style={labelStyle}>Snapshot B (later)</label>
              <select value={diffB} onChange={e => setDiffB(e.target.value)} style={{ ...inputStyle(), appearance: 'none' }}>
                <option value="">Select snapshot…</option>
                {snapshots.map(s => (
                  <option key={s.id} value={s.id}>{s.shortId} — {new Date(s.time).toLocaleString()}</option>
                ))}
              </select>
            </div>
            <Button variant="primary" onClick={handleDiff} disabled={!diffA || !diffB}>Compare</Button>
          </div>
          {diffResults && (
            <div style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-1)' }}>
              {diffResults.length === 0 ? (
                <div style={{ color: 'var(--color-text-muted)', fontSize: '13px' }}>No differences found.</div>
              ) : diffResults.map((d, i) => (
                <div key={i} style={{ display: 'flex', alignItems: 'center', gap: 'var(--space-3)', padding: 'var(--space-2) var(--space-3)', borderRadius: 'var(--radius-sm)', background: diffColor(d.changeType, 0.07) }}>
                  <span style={{ fontSize: '11px', fontFamily: 'var(--font-mono)', fontWeight: 600, color: diffColor(d.changeType, 1), width: 50, flexShrink: 0 }}>
                    {d.changeType.toUpperCase()}
                  </span>
                  {d.isSealed && <LockIcon size={12} color="var(--color-text-muted)" />}
                  <span style={{ fontFamily: 'var(--font-mono)', fontSize: '12px', flex: 1 }}>{d.path}</span>
                  {d.isSealed && <span style={{ fontSize: '11px', color: 'var(--color-text-muted)' }}>sealed</span>}
                </div>
              ))}
            </div>
          )}
        </div>
      )}
    </div>
  )
}

function diffColor(changeType, alpha) {
  const colors = {
    added:   `rgba(61,187,114,${alpha})`,
    removed: `rgba(224,80,80,${alpha})`,
    changed: `rgba(232,160,48,${alpha})`,
  }
  return colors[changeType] ?? `rgba(100,112,160,${alpha})`
}

function TreeView({ nodes }) {
  const [expanded, setExpanded] = useState(new Set())

  const toggle = (path) => setExpanded(s => {
    const n = new Set(s)
    if (n.has(path)) n.delete(path); else n.add(path)
    return n
  })

  if (nodes.length === 0) return <div style={{ color: 'var(--color-text-muted)', fontSize: '13px' }}>Empty snapshot.</div>

  return (
    <div style={{ fontFamily: 'var(--font-mono)', fontSize: '13px' }}>
      {nodes.map(node => (
        <TreeNode key={node.path} node={node} expanded={expanded} onToggle={toggle} depth={0} />
      ))}
    </div>
  )
}

function TreeNode({ node, expanded, onToggle, depth }) {
  const isDir = node.type === 'dir'
  const isExpanded = expanded.has(node.path)

  return (
    <div>
      <div
        onClick={() => isDir && onToggle(node.path)}
        style={{
          display: 'flex', alignItems: 'center', gap: 'var(--space-2)',
          padding: '3px 0', paddingLeft: depth * 16,
          cursor: isDir ? 'pointer' : 'default',
          color: node.isSealed ? 'var(--color-text-muted)' : 'var(--color-text-primary)',
        }}
      >
        {isDir
          ? (isExpanded ? <ChevronDownIcon size={12} /> : <ChevronRightIcon size={12} />)
          : <span style={{ width: 12 }} />
        }
        {node.isSealed
          ? <LockIcon size={13} color="var(--color-text-muted)" />
          : isDir ? <FolderIcon size={13} color="var(--color-text-muted)" /> : <FileIcon size={13} color="var(--color-text-muted)" />
        }
        <span>{node.name}</span>
        {node.isSealed && <span style={{ fontSize: '11px', color: 'var(--color-text-muted)', marginLeft: 'var(--space-2)' }}>(sealed)</span>}
      </div>
    </div>
  )
}

const labelStyle = { display: 'block', fontSize: '12px', color: 'var(--color-text-muted)', marginBottom: 'var(--space-2)' }
function inputStyle() {
  return {
    width: '100%', padding: '8px 12px', fontFamily: 'var(--font-ui)', fontSize: '13px',
    background: 'var(--color-bg-primary)', border: '1px solid var(--color-border)',
    borderRadius: 'var(--radius-sm)', color: 'var(--color-text-primary)', outline: 'none',
  }
}
