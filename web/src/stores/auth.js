import { defineStore } from 'pinia'

const STORAGE_KEY = 'gateway_admin_token'

export const useAuthStore = defineStore('auth', {
  state: () => ({
    token: localStorage.getItem(STORAGE_KEY) || ''
  }),
  getters: {
    isAuthenticated: (state) => Boolean(state.token)
  },
  actions: {
    setToken(token) {
      this.token = token
      localStorage.setItem(STORAGE_KEY, token)
    },
    clearToken() {
      this.token = ''
      localStorage.removeItem(STORAGE_KEY)
    }
  }
})

export function getStoredToken() {
  return localStorage.getItem(STORAGE_KEY) || ''
}
