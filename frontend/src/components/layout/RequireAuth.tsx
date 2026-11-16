import { Navigate, Outlet, useLocation } from 'react-router-dom'
import { useMe } from '../../hooks/useMe'
import { PageLoader } from '../ui/Feedback'

export function RequireAuth() {
  const { data: me, isLoading } = useMe()
  const location = useLocation()
  if (isLoading) return <PageLoader />
  if (!me) return <Navigate to="/login" state={{ from: location.pathname }} replace />
  return <Outlet />
}
