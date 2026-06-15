import Sidebar from './Sidebar.jsx'
import StatusStrip from './StatusStrip.jsx'
import AlertBanner from './AlertBanner.jsx'

/**
 * Shell layout: [Sidebar] | [AlertBanner + main content] / [StatusStrip]
 *
 * @param {string}   screen      - current active screen id
 * @param {function} onNavigate  - called with screen id on nav click
 * @param {Array}    alerts      - alert objects [{grade, message, action}]
 * @param {object}   health      - status result from GetStatus()
 * @param {string}   lastBackup  - formatted last backup time string
 * @param {*}        children    - the active screen component
 */
export default function Layout({ screen, onNavigate, alerts, health, lastBackup, children }) {
  return (
    <div
      style={{
        display: 'flex',
        flexDirection: 'column',
        height: '100%',
        overflow: 'hidden',
      }}
    >
      <div style={{ display: 'flex', flex: 1, overflow: 'hidden' }}>
        <Sidebar currentScreen={screen} onNavigate={onNavigate} />
        <div style={{ flex: 1, display: 'flex', flexDirection: 'column', overflow: 'hidden' }}>
          <AlertBanner alerts={alerts} />
          <main
            id="main-content"
            tabIndex={-1}
            style={{ flex: 1, overflowY: 'auto', padding: 'var(--space-6)' }}
          >
            {children}
          </main>
        </div>
      </div>
      <StatusStrip health={health} lastBackup={lastBackup} />
    </div>
  )
}
