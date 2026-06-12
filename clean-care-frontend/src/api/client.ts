import { ApiError } from '../types/api';
import type { Envelope } from '../types/api';
import { getAdminAPIKey, getAuthToken } from '../auth/token';

const BASE_URL = '/api/v1';
const REQUEST_TIMEOUT_MS = 60_000;

interface FetchOptions extends Omit<RequestInit, 'body'> {
  body?: unknown;
}

/** Fetch wrapper: prepends base URL, adds auth, parses envelope, throws on error */
export async function apiFetch<T>(
  path: string,
  options: FetchOptions = {}
): Promise<T> {
  const { body, headers, ...rest } = options;

  const fetchHeaders: Record<string, string> = {
    Authorization: getAuthToken(),
    ...(headers as Record<string, string>),
  };
  if (path.startsWith('/admin/')) {
    const adminKey = getAdminAPIKey();
    if (adminKey) fetchHeaders['X-Admin-API-Key'] = adminKey;
  }

  let fetchBody: string | FormData | undefined;
  if (body !== undefined) {
    if (body instanceof FormData) {
      fetchBody = body;
      // Let browser set Content-Type with boundary for FormData
    } else {
      fetchHeaders['Content-Type'] = 'application/json';
      fetchBody = JSON.stringify(body);
    }
  }

  // Request timeout via AbortController
  const controller = new AbortController();
  const timeoutId = setTimeout(() => controller.abort(), REQUEST_TIMEOUT_MS);

  const existingSignal = rest.signal;
  if (existingSignal) {
    if (existingSignal.aborted) controller.abort();
    else existingSignal.addEventListener('abort', () => controller.abort());
  }

  try {
    const response = await fetch(`${BASE_URL}${path}`, {
      ...rest,
      headers: fetchHeaders,
      body: fetchBody,
      signal: controller.signal,
    });

    clearTimeout(timeoutId);

    if (!response.ok) {
      let code = 'HTTP_ERROR';
      let message = `HTTP ${response.status}`;
      let requestId: string | undefined;
      try {
        const errBody = await response.json();
        code = errBody.code || code;
        message = errBody.message || message;
        requestId = errBody.request_id;
      } catch {
        // Use defaults
      }
      throw new ApiError(response.status, code, message, requestId);
    }

    const envelope: Envelope<T> = await response.json();

    if (envelope.code && envelope.code !== 'OK') {
      throw new ApiError(
        response.status,
        envelope.code,
        envelope.message || 'Unknown error',
        envelope.request_id
      );
    }

    return envelope.data;
  } catch (err) {
    clearTimeout(timeoutId);
    if ((err as Error).name === 'AbortError' && !existingSignal?.aborted) {
      throw new ApiError(0, 'TIMEOUT', 'Request timed out', undefined);
    }
    throw err;
  }
}

/** Convenience: GET request */
export function apiGet<T>(path: string): Promise<T> {
  return apiFetch<T>(path, { method: 'GET' });
}

/** Convenience: POST request */
export function apiPost<T>(path: string, body?: unknown): Promise<T> {
  return apiFetch<T>(path, { method: 'POST', body });
}
