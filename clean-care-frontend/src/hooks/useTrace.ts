import { useState, useEffect, useCallback } from 'react';
import { getTrace } from '../api/traces';
import type { AgentTraceRecord } from '../types/trace';
import { ApiError } from '../types/api';

export default function useTrace(traceId: string | undefined) {
  const [trace, setTrace] = useState<AgentTraceRecord | null>(null);
  const [isLoading, setIsLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const fetch = useCallback(() => {
    if (!traceId) return;
    setIsLoading(true);
    setError(null);
    getTrace(traceId)
      .then(setTrace)
      .catch((err) => setError(err instanceof ApiError ? err.message : 'Failed to load trace'))
      .finally(() => setIsLoading(false));
  }, [traceId]);

  useEffect(() => {
    fetch();
  }, [fetch]);

  return { trace, isLoading, error, refetch: fetch };
}
