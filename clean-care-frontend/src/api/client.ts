import { ApiError } from '../types/api';
import type { Envelope } from '../types/api';
import { getAdminAPIKey, getAuthToken } from '../auth/token';
import { recordApiMetric } from '../utils/perfMonitor';

const BASE_URL = '/api/v1';
const REQUEST_TIMEOUT_MS = 60_000;
const MAX_RETRIES = 3;
const RETRY_DELAY_BASE = 1000; // 1 second

interface FetchOptions extends Omit<RequestInit, 'body'> {
  body?: unknown;
  retries?: number;
  cache?: RequestCache;
}

const requestCache = new Map<string, { data: unknown; timestamp: number }>();
const CACHE_TTL = 5 * 60 * 1000; // 5 minutes

// Helper: sleep for retry delay
const sleep = (ms: number) => new Promise(resolve => setTimeout(resolve, ms));

// Helper: check if error is retryable
const isRetryable = (error: unknown): boolean => {
  if (error instanceof ApiError) {
    // Retry on network errors, timeouts, and 5xx server errors
    return (
      error.code === 'TIMEOUT' ||
      error.code === 'NETWORK_ERROR' ||
      error.statusCode >= 500
    );
  }
  return false;
};

// Helper: get retry delay with exponential backoff
const getRetryDelay = (attempt: number): number => {
  return RETRY_DELAY_BASE * Math.pow(2, attempt) + Math.random() * 1000;
};

// Helper: generate cache key
const getCacheKey = (path: string, options: FetchOptions): string => {
  const method = options.method || 'GET';
  const authScope = getAuthToken() ? 'auth' : 'anonymous';
  const adminScope = path.startsWith('/admin/') && getAdminAPIKey() ? 'admin' : 'user';
  return `${authScope}:${adminScope}:${method}:${path}`;
};

const isMemoryCacheAllowed = (path: string, method: string, cache?: RequestCache): boolean => {
  if (method !== 'GET' || cache !== 'force-cache') return false;
  return !(
    path.startsWith('/admin/') ||
    path.startsWith('/conversations') ||
    path.startsWith('/orders') ||
    path.startsWith('/after-sales')
  );
};

// Clear expired cache entries
const cleanupCache = () => {
  const now = Date.now();
  for (const [key, entry] of requestCache.entries()) {
    if (now - entry.timestamp > CACHE_TTL) {
      requestCache.delete(key);
    }
  }
};

// Run cache cleanup periodically
setInterval(cleanupCache, 60_000);

/** Fetch wrapper: prepends base URL, adds auth, parses envelope, throws on error */
export async function apiFetch<T>(
  path: string,
  options: FetchOptions = {}
): Promise<T> {
  const { body, headers, retries = MAX_RETRIES, cache, ...rest } = options;
  const method = options.method || 'GET';
  const cacheKey = getCacheKey(path, options);

  const useMemoryCache = isMemoryCacheAllowed(path, method, cache);

  if (useMemoryCache) {
    const cached = requestCache.get(cacheKey);
    if (cached && Date.now() - cached.timestamp < CACHE_TTL) {
      recordApiMetric({
        path,
        method,
        status: 200,
        duration: 0,
        cached: true,
      });
      return cached.data as T;
    }
  }

  let lastError: unknown;

  for (let attempt = 0; attempt <= retries; attempt++) {
    try {
      const result = await executeFetch<T>(path, { body, headers, ...rest });

      if (useMemoryCache) {
        requestCache.set(cacheKey, {
          data: result,
          timestamp: Date.now(),
        });
      }

      // Invalidate related GET cache on mutations
      if (method === 'POST' || method === 'PUT' || method === 'DELETE') {
        clearCacheForPath(path);
      }

      return result;
    } catch (error) {
      lastError = error;

      // Don't retry if not retryable or last attempt
      if (!isRetryable(error) || attempt === retries) {
        throw error;
      }

      // Wait before retry
      const delay = getRetryDelay(attempt);
      console.warn(`Request failed, retrying in ${delay}ms (attempt ${attempt + 1}/${retries}):`, error);
      await sleep(delay);
    }
  }

  throw lastError;
}

/** Internal fetch implementation */
async function executeFetch<T>(
  path: string,
  options: Omit<FetchOptions, 'retries' | 'cache'>
): Promise<T> {
  const { body, headers, ...rest } = options;

  const fetchHeaders: Record<string, string> = {
    ...(headers as Record<string, string>),
  };
  const authToken = getAuthToken();
  if (authToken) fetchHeaders.Authorization = authToken;

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

  const method = rest.method || 'GET';
  const startTime = performance.now();

  try {
    const response = await fetch(`${BASE_URL}${path}`, {
      ...rest,
      headers: fetchHeaders,
      body: fetchBody,
      signal: controller.signal,
      cache: 'no-store',
    });

    clearTimeout(timeoutId);
    const duration = performance.now() - startTime;

    // Record API metric
    recordApiMetric({
      path,
      method,
      status: response.status,
      duration,
      cached: false,
    });

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
    const duration = performance.now() - startTime;

    // Record failed request metric
    recordApiMetric({
      path,
      method,
      status: (err instanceof ApiError && err.statusCode) || 0,
      duration,
      cached: false,
    });

    if ((err as Error).name === 'AbortError' && !existingSignal?.aborted) {
      throw new ApiError(0, 'TIMEOUT', 'Request timed out', undefined);
    }

    // Wrap network errors
    if (err instanceof TypeError && err.message === 'Failed to fetch') {
      throw new ApiError(0, 'NETWORK_ERROR', 'Network error, please check your connection', undefined);
    }

    throw err;
  }
}

/** Convenience: GET request */
export function apiGet<T>(path: string, options?: Omit<FetchOptions, 'method'>): Promise<T> {
  return apiFetch<T>(path, { ...options, method: 'GET' });
}

/** Convenience: POST request */
export function apiPost<T>(path: string, body?: unknown, options?: Omit<FetchOptions, 'method' | 'body'>): Promise<T> {
  return apiFetch<T>(path, { ...options, method: 'POST', body });
}

/** Convenience: PUT request */
export function apiPut<T>(path: string, body?: unknown, options?: Omit<FetchOptions, 'method' | 'body'>): Promise<T> {
  return apiFetch<T>(path, { ...options, method: 'PUT', body });
}

/** Convenience: DELETE request */
export function apiDelete<T>(path: string, options?: Omit<FetchOptions, 'method'>): Promise<T> {
  return apiFetch<T>(path, { ...options, method: 'DELETE' });
}

/** Clear the entire request cache */
export function clearCache(): void {
  requestCache.clear();
}

/** Clear cache for a specific path */
export function clearCacheForPath(path: string): void {
  const keysToDelete = Array.from(requestCache.keys()).filter(key => key.includes(path));
  keysToDelete.forEach(key => requestCache.delete(key));
}
