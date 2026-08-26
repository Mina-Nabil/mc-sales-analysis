import React from 'react'
import { createRoot } from 'react-dom/client'
import { BrowserRouter, Routes, Route, Navigate } from 'react-router-dom'
import { ThemeProvider } from '@/theme/ThemeProvider'
import { AuthProvider, useAuth } from '@/auth'
import { AppShell } from '@/layout/AppShell'
import Login from '@/pages/Login'
import Overview from '@/pages/Overview'
import Analytics from '@/pages/Analytics'
import ModelAnalytics from '@/pages/ModelAnalytics'
import Dashboard from '@/pages/Dashboard'
import Review from '@/pages/Review'
import Tree from '@/pages/Tree'
import Import from '@/pages/Import'
import Settings from '@/pages/Settings'
import Audit from '@/pages/Audit'
import './index.css'

function Protected({ children }: { children: React.ReactNode }) {
  const { user, loading } = useAuth()
  if (loading) return <div className="grid min-h-screen place-items-center bg-bg-0 text-t1">Loading…</div>
  if (!user) return <Navigate to="/login" replace />
  return <>{children}</>
}

createRoot(document.getElementById('root')!).render(
  <React.StrictMode>
    <ThemeProvider>
      <BrowserRouter>
        <AuthProvider>
          <Routes>
            <Route path="/login" element={<Login />} />
            <Route path="/" element={<Protected><AppShell /></Protected>}>
              <Route index element={<Overview />} />
              <Route path="analytics" element={<Analytics />} />
              <Route path="analytics/view/:id" element={<Analytics />} />
              <Route path="models" element={<ModelAnalytics />} />
              <Route path="models/view/:id" element={<ModelAnalytics />} />
              <Route path="dashboard" element={<Dashboard />} />
              <Route path="review" element={<Review />} />
              <Route path="tree" element={<Tree />} />
              <Route path="import" element={<Import />} />
              <Route path="settings" element={<Settings />} />
              <Route path="audit" element={<Audit />} />
            </Route>
            <Route path="*" element={<Navigate to="/" replace />} />
          </Routes>
        </AuthProvider>
      </BrowserRouter>
    </ThemeProvider>
  </React.StrictMode>,
)
