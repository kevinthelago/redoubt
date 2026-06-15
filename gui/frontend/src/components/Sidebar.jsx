import {
  HomeIcon, ShieldIcon, ArchiveIcon, CameraIcon, RotateCcwIcon,
  HardDriveIcon, CheckCircleIcon, KeyIcon, ServerIcon, FolderIcon,
  SettingsIcon, WifiOffIcon,
} from '../icons/index.jsx'

const NAV_ITEMS = [
  { id: 'dashboard',  label: 'Dashboard',  Icon: HomeIcon        },
  { id: 'health',     label: 'Health',     Icon: ShieldIcon      },
  { id: 'backups',    label: 'Backups',    Icon: ArchiveIcon     },
  { id: 'snapshots',  label: 'Snapshots',  Icon: CameraIcon      },
  { id: 'restore',    label: 'Restore',    Icon: RotateCcwIcon   },
  { id: 'cold',       label: 'Cold Copy',  Icon: HardDriveIcon   },
  { id: 'drills',     label: 'Drills',     Icon: CheckCircleIcon },
  { id: 'keys',       label: 'Keys',       Icon: KeyIcon         },
  { id: 'vault',      label: 'Vault',      Icon: ServerIcon      },
  { id: 'assets',     label: 'Assets',     Icon: FolderIcon      },
  { id: 'settings',   label: 'Settings',   Icon: SettingsIcon    },
]

/**
 * Navigation sidebar. Current screen is highlighted.
 * Keyboard navigable via Enter/Space.
 */
export default function Sidebar({ currentScreen, onNavigate }) {
  const handleKeyDown = (e, id) => {
    if (e.key === 'Enter' || e.key === ' ') {
      e.preventDefault()
      onNavigate(id)
    }
  }

  return (
    <nav
      aria-label="Main navigation"
      style={{
        width: 200,
        flexShrink: 0,
        background: 'var(--color-bg-secondary)',
        borderRight: '1px solid var(--color-border)',
        display: 'flex',
        flexDirection: 'column',
        overflow: 'hidden',
      }}
    >
      {/* App title */}
      <div style={{
        padding: 'var(--space-4) var(--space-4) var(--space-3)',
        borderBottom: '1px solid var(--color-border-subtle)',
      }}>
        <div style={{ fontWeight: 700, fontSize: '15px', letterSpacing: '-0.01em' }}>Redoubt</div>
        <div style={{ fontSize: '11px', color: 'var(--color-text-muted)', marginTop: 'var(--space-1)' }}>Backup management</div>
      </div>

      {/* Nav items */}
      <ul
        role="list"
        style={{ flex: 1, overflowY: 'auto', padding: 'var(--space-2) 0', margin: 0, listStyle: 'none' }}
      >
        {NAV_ITEMS.map(({ id, label, Icon }) => {
          const isActive = currentScreen === id
          return (
            <li key={id}>
              <button
                role="menuitem"
                aria-current={isActive ? 'page' : undefined}
                onClick={() => onNavigate(id)}
                onKeyDown={(e) => handleKeyDown(e, id)}
                style={{
                  display: 'flex',
                  alignItems: 'center',
                  gap: 'var(--space-3)',
                  width: '100%',
                  padding: '7px var(--space-4)',
                  background: isActive ? 'var(--color-surface-active)' : 'transparent',
                  color: isActive ? 'var(--color-text-primary)' : 'var(--color-text-secondary)',
                  border: 'none',
                  borderLeft: isActive ? '2px solid var(--color-accent)' : '2px solid transparent',
                  cursor: 'pointer',
                  fontSize: '13px',
                  fontFamily: 'var(--font-ui)',
                  fontWeight: isActive ? 500 : 400,
                  textAlign: 'left',
                  transition: `background var(--transition-fast), color var(--transition-fast)`,
                }}
              >
                <Icon size={15} />
                {label}
              </button>
            </li>
          )
        })}
      </ul>

      {/* LAN-only badge */}
      <div style={{
        padding: 'var(--space-3) var(--space-4)',
        borderTop: '1px solid var(--color-border-subtle)',
        display: 'flex',
        alignItems: 'center',
        gap: 'var(--space-2)',
        color: 'var(--color-text-muted)',
        fontSize: '11px',
      }}>
        <WifiOffIcon size={11} />
        <span>LAN-only · offline</span>
      </div>
    </nav>
  )
}
