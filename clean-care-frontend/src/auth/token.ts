let authToken = '';

function normalizeBearerToken(token: string): string {
  const trimmed = token.trim();
  if (!trimmed) return '';
  return trimmed.toLowerCase().startsWith('bearer ') ? trimmed : `Bearer ${trimmed}`;
}

export function getAuthToken(): string {
  return authToken;
}

export function setAuthToken(token: string): void {
  authToken = normalizeBearerToken(token);
}

export function clearAuthToken(): void {
  authToken = '';
}
