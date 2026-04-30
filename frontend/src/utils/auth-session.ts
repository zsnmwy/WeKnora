import { nextTick } from 'vue'
import type { Router } from 'vue-router'
import { getCurrentUser } from '@/api/auth'
import type { useAuthStore } from '@/stores/auth'

export const DEFAULT_AUTH_REDIRECT = '/platform/knowledge-bases'

type AuthStore = ReturnType<typeof useAuthStore>

export async function syncAuthenticatedUserContext(authStore: AuthStore) {
  const currentUserResponse = await getCurrentUser()
  if (!currentUserResponse.success || !currentUserResponse.data?.user || !currentUserResponse.data?.tenant) {
    throw new Error(currentUserResponse.message || 'Failed to get user information')
  }

  const { user, tenant } = currentUserResponse.data
  authStore.setUser({
    id: user.id || '',
    username: user.username || '',
    email: user.email || '',
    avatar: user.avatar,
    tenant_id: String(user.tenant_id || tenant.id || ''),
    can_access_all_tenants: user.can_access_all_tenants || false,
    created_at: user.created_at || new Date().toISOString(),
    updated_at: user.updated_at || new Date().toISOString()
  })
  authStore.setTenant({
    id: String(tenant.id) || '',
    name: tenant.name || '',
    api_key: tenant.api_key || '',
    owner_id: tenant.owner_id || user.id || '',
    description: tenant.description,
    status: tenant.status,
    business: tenant.business,
    storage_quota: tenant.storage_quota,
    storage_used: tenant.storage_used,
    created_at: tenant.created_at || new Date().toISOString(),
    updated_at: tenant.updated_at || new Date().toISOString()
  })
}

export async function persistLoginSession(authStore: AuthStore, response: any) {
  if (!response?.token) {
    throw new Error(response?.message || 'Login failed: missing access token')
  }

  authStore.setToken(response.token)
  if (response.refresh_token) {
    authStore.setRefreshToken(response.refresh_token)
  }

  await syncAuthenticatedUserContext(authStore)
  await nextTick()
}

export async function redirectAfterLogin(router: Router, path = DEFAULT_AUTH_REDIRECT) {
  try {
    await router.replace(path)
  } catch (error) {
    console.error('Failed to navigate after login:', error)
    if (typeof window !== 'undefined') {
      window.location.assign(path)
    }
  }
}
