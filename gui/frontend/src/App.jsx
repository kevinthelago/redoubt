import { useCallback, useEffect, useState } from 'react'
import Layout from './components/Layout.jsx'

// Screens
import Dashboard from './screens/Dashboard.jsx'
import Health from './screens/Health.jsx'
import Backups from './screens/Backups.jsx'
import Snapshots from './screens/Snapshots.jsx'
import Restore from './screens/Restore.jsx'
import ColdCopy from './screens/ColdCopy.jsx'
import Drills from './screens/Drills.jsx'
import Keys from './screens/Keys.jsx'
import Vault from './screens/Vault.jsx'
import Assets from './screens/Assets.jsx'
import Settings from './screens/Settings.jsx'

const SCREENS = {
  dashboard: Dashboard,
  health:    Health,
  backups:   Backups,
  snapshots: Snapshots,
  restore:   Restore,
  cold:      ColdCopy,
  drills:    Drills,
  keys:      Keys,
  vault:     Vault,
  assets:    Assets,
  settings:  Settings,
}

export default function App() {
  const [screen, setScreen] = useState('dashboard')
  const [theme, setTheme] = useState('dark')
  const [health, setHealth] = useState(null)
  const [alerts, setAlerts] = useState([])

  // Apply theme to root element.
  useEffect(() => {
    document.documentElement.setAttribute('data-theme', theme)
  }, [theme])

  // Load theme preference from Go backend on startup.
  useEffect(() => {
    let cancelled = false
    // Wails bindings may not be available until runtime is ready.
    // We try after a short delay to let Wails inject the runtime.
    const load = async () => {
      try {
        const { GetTheme } = await import('../wailsjs/go/main/App.js')
        if (!cancelled) {
          const t = await GetTheme()
          if (!cancelled && t) setTheme(t)
        }
      } catch {
        // Wails not ready or running in browser dev mode — keep default.
      }
    }
    const timer = setTimeout(load, 100)
    return () => { cancelled = true; clearTimeout(timer) }
  }, [])

  // Poll health every 30s and derive alerts.
  useEffect(() => {
    let cancelled = false
    const fetchHealth = async () => {
      try {
        const { GetStatus } = await import('../wailsjs/go/main/App.js')
        const status = await GetStatus()
        if (!cancelled) {
          setHealth(status)
          setAlerts(deriveAlerts(status))
        }
      } catch {
        // Ignore — status strip shows unknown.
      }
    }
    fetchHealth()
    const id = setInterval(fetchHealth, 30_000)
    return () => { cancelled = true; clearInterval(id) }
  }, [])

  const handleNavigate = useCallback((id) => setScreen(id), [])

  const handleThemeChange = useCallback(async (t) => {
    setTheme(t)
    try {
      const { SetTheme } = await import('../wailsjs/go/main/App.js')
      await SetTheme(t)
    } catch { /* ignore */ }
  }, [])

  const Screen = SCREENS[screen] ?? Dashboard

  const lastBackup = health?.signals?.last_backup?.message ?? null

  return (
    <Layout
      screen={screen}
      onNavigate={handleNavigate}
      alerts={alerts}
      health={health}
      lastBackup={lastBackup}
    >
      <Screen
        onNavigate={handleNavigate}
        health={health}
        theme={theme}
        onThemeChange={handleThemeChange}
      />
    </Layout>
  )
}

/** Derive alert banners from the status result. */
function deriveAlerts(status) {
  if (!status?.signals) return []
  return Object.entries(status.signals)
    .filter(([, sig]) => sig.grade === 'warning' || sig.grade === 'critical')
    .map(([key, sig]) => ({
      grade: sig.grade,
      message: sig.message,
      action: actionForSignal(key, sig.grade),
    }))
}

function actionForSignal(key, grade) {
  const actions = {
    last_backup: grade === 'critical' ? 'Back up now →' : 'Schedule a backup →',
    drill:       'Run a restore drill →',
    cold_copy:   'Make a cold copy →',
    vault:       'Check vault connectivity →',
    disk:        'Free up space →',
    key:         'Verify key escrow →',
  }
  return actions[key] ?? null
}
