import { Navigate, Route, Routes } from 'react-router-dom'
import { AppShell } from './components/layout/AppShell'
import { RequireAuth } from './components/layout/RequireAuth'
import { RequireHost } from './components/layout/RequireHost'
import { AdminPage } from './pages/admin/AdminPage'
import { CheckInPage } from './pages/host/CheckInPage'
import { EventWizard } from './pages/host/EventWizard'
import { HostDashboard } from './pages/host/HostDashboard'
import { HostEventPage } from './pages/host/HostEventPage'
import { BookingPage } from './pages/public/BookingPage'
import { BookingsPage } from './pages/public/BookingsPage'
import { CheckoutPage } from './pages/public/CheckoutPage'
import { EventPage } from './pages/public/EventPage'
import { HomePage } from './pages/public/HomePage'
import { LoginPage } from './pages/public/LoginPage'

export default function App() {
  return (
    <Routes>
      <Route element={<AppShell />}>
        <Route index element={<HomePage />} />
        <Route path="login" element={<LoginPage />} />
        <Route path="events/:id" element={<EventPage />} />

        <Route element={<RequireAuth />}>
          <Route path="checkout/:id" element={<CheckoutPage />} />
          <Route path="bookings" element={<BookingsPage />} />
          <Route path="bookings/:id" element={<BookingPage />} />
          <Route path="admin" element={<AdminPage />} />
        </Route>

        <Route element={<RequireHost />}>
          <Route path="host" element={<HostDashboard />} />
          <Route path="host/events/new" element={<EventWizard key="new" />} />
          <Route path="host/events/:id" element={<HostEventPage />} />
          <Route path="host/events/:id/edit" element={<EventWizard />} />
          <Route path="host/check-in" element={<CheckInPage />} />
          <Route path="check-in/:code" element={<CheckInPage />} />
        </Route>

        <Route path="*" element={<Navigate to="/" replace />} />
      </Route>
    </Routes>
  )
}
