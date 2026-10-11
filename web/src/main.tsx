import React from 'react'
import ReactDOM from 'react-dom/client'
import { BrowserRouter, Navigate, Route, Routes, useLocation } from 'react-router-dom'
import './styles.css'
import { AppProvider, useApp } from './lib/app-context'
import { Loading, ToastProvider } from './components/ui'
import Layout from './components/Layout'
import Login from './pages/Login'
import Now from './pages/Now'
import Library from './pages/Library'
import Book from './pages/Book'
import Planner from './pages/Planner'
import Quotes from './pages/Quotes'
import Stats from './pages/Stats'
import Authors from './pages/Authors'
import Daily from './pages/Daily'
import Series from './pages/Series'
import SeriesDetail from './pages/SeriesDetail'
import Author from './pages/Author'
import { AuthorByName } from './components/AuthorBits'
import Settings from './pages/Settings'

function Gate({ children }: { children: React.ReactNode }) {
  const { status, user } = useApp()
  const loc = useLocation()
  if (!status) return <div className="login-wrap"><Loading /></div>
  if (status.needs_setup) return <Navigate to="/setup" replace />
  if (!user) return <Navigate to="/login" replace state={{ from: loc.pathname }} />
  return <>{children}</>
}

function Landing() {
  const { settings } = useApp()
  const to = settings?.landing === 'library' ? '/library' : settings?.landing === 'planner' ? '/planner' : '/now'
  return <Navigate to={to} replace />
}

function App() {
  return (
    <Routes>
      <Route path="/login" element={<Login mode="login" />} />
      <Route path="/setup" element={<Login mode="setup" />} />
      <Route element={<Gate><Layout /></Gate>}>
        <Route path="/" element={<Landing />} />
        <Route path="/now" element={<Now />} />
        <Route path="/library" element={<Library />} />
        <Route path="/books/:id" element={<Book />} />
        <Route path="/planner" element={<Planner />} />
        <Route path="/quotes" element={<Quotes />} />
        <Route path="/authors" element={<Authors />} />
        <Route path="/daily" element={<Daily />} />
        <Route path="/series" element={<Series />} />
        <Route path="/series/:name" element={<SeriesDetail />} />
        <Route path="/authors/:id" element={<Author />} />
        <Route path="/author/:name" element={<AuthorByName />} />
        <Route path="/stats" element={<Stats />} />
        <Route path="/settings" element={<Settings />} />
        <Route path="*" element={<Navigate to="/" replace />} />
      </Route>
    </Routes>
  )
}

ReactDOM.createRoot(document.getElementById('root')!).render(
  <React.StrictMode>
    <BrowserRouter>
      <ToastProvider>
        <AppProvider>
          <App />
        </AppProvider>
      </ToastProvider>
    </BrowserRouter>
  </React.StrictMode>,
)
