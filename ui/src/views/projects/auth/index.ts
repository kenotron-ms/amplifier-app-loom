// Auth is disabled in loom — no-op stubs
export function useAuthStore() { return { user: null, isAuthenticated: true } }
export function LoginPage() { return null }
export function OAuthCallback() { return null }
export function ProtectedRoute({ children }: { children: React.ReactNode }) { return children }
import React from 'react'
