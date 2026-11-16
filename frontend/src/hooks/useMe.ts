import { useQuery } from '@tanstack/react-query'
import { ApiError } from '../api/client'
import { authApi } from '../api/endpoints'
import type { User } from '../lib/types'

export const ME_KEY = ['me'] as const

/** Current user, or null when signed out. */
export function useMe() {
  return useQuery<User | null>({
    queryKey: ME_KEY,
    queryFn: async () => {
      try {
        return await authApi.me()
      } catch (e) {
        if (e instanceof ApiError && e.status === 401) return null
        throw e
      }
    },
    staleTime: 5 * 60_000,
  })
}
