const TOKEN_KEY = 'cleancare_auth_token';
const ADMIN_KEY = 'cleancare_admin_api_key';
const MOCK_TOKEN = 'Bearer mock-jwt-demo-token';

/** Read the stored auth token, falling back to the mock demo token. */
export function getAuthToken(): string {
  try {
    const stored = localStorage.getItem(TOKEN_KEY);
    if (stored) return stored;
  } catch {
    // localStorage unavailable (SSR, privacy mode) — use mock
  }
  return MOCK_TOKEN;
}

/** Persist an auth token (e.g. after login). */
export function setAuthToken(token: string): void {
  try {
    localStorage.setItem(TOKEN_KEY, token);
  } catch {
    // Silently ignore — auth will fall back to the mock token
  }
}

/** Remove the stored token (logout). */
export function clearAuthToken(): void {
  try {
    localStorage.removeItem(TOKEN_KEY);
  } catch {
    // Silently ignore
  }
}

/** Read the administrator API key used only for /admin requests. */
export function getAdminAPIKey(): string {
  try {
    return localStorage.getItem(ADMIN_KEY) || '';
  } catch {
    return '';
  }
}

/** Persist the administrator API key for the current browser profile. */
export function setAdminAPIKey(value: string): void {
  try {
    localStorage.setItem(ADMIN_KEY, value.trim());
  } catch {
    // The UI remains usable when authentication is disabled locally.
  }
}
