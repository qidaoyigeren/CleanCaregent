import { useState, useCallback } from 'react';
import { getAuthToken, setAuthToken, clearAuthToken } from '../auth/token';

interface AuthState {
  isAuthenticated: boolean;
  token: string | null;
}

/** Simple auth hook for managing authentication state */
export function useAuth() {
  const [state, setState] = useState<AuthState>(() => {
    const token = getAuthToken();
    return {
      isAuthenticated: token !== '',
      token: token || null,
    };
  });

  const login = useCallback((token: string) => {
    setAuthToken(token);
    setState({ isAuthenticated: true, token });
  }, []);

  const logout = useCallback(() => {
    clearAuthToken();
    setState({ isAuthenticated: false, token: null });
  }, []);

  return { ...state, login, logout };
}
