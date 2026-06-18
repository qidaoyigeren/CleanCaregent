import { useCallback, useRef, useState, useEffect } from 'react';
import type { StatusEvent, EvidenceEvent, DoneEvent, SSEErrorEvent } from '../types/sse';
import { getAuthToken } from '../auth/token';

// Type for setTimeout return value
type TimeoutId = ReturnType<typeof setTimeout>;

export type ConnectionState = 'idle' | 'connecting' | 'connected' | 'reconnecting' | 'disconnected' | 'error';

interface SSEEventHandler {
  onStatus?: (data: StatusEvent) => void;
  onEvidence?: (data: EvidenceEvent) => void;
  onDelta?: (content: string) => void;
  onDone?: (data: DoneEvent) => void;
  onError?: (error: SSEErrorEvent) => void;
}

interface SSEStreamResult {
  startStream: (conversationId: string, content: string, handlers: SSEEventHandler) => Promise<void>;
  abort: () => void;
  isStreaming: boolean;
  connectionState: ConnectionState;
  reconnectCount: number;
}

export default function useSSEStream(): SSEStreamResult {
  const [isStreaming, setIsStreaming] = useState(false);
  const [connectionState, setConnectionState] = useState<ConnectionState>('idle');
  const [reconnectCount, setReconnectCount] = useState(0);
  const abortRef = useRef<AbortController | null>(null);
  const heartbeatRef = useRef<TimeoutId | null>(null);
  const lastActivityRef = useRef<number>(Date.now());
  // Ref to avoid stale closure in finally block
  const connectionStateRef = useRef<ConnectionState>('idle');
  // Ref to track if stream is active (for heartbeat reconnection)
  const streamActiveRef = useRef(false);
  // Ref to hold the current fetch abort for heartbeat-triggered reconnect
  const reconnectHandlerRef = useRef<(() => void) | null>(null);

  // Keep ref in sync with state
  useEffect(() => {
    connectionStateRef.current = connectionState;
  }, [connectionState]);

  // Heartbeat detection with active reconnection
  const startHeartbeat = useCallback(() => {
    if (heartbeatRef.current) {
      clearInterval(heartbeatRef.current);
    }
    lastActivityRef.current = Date.now();

    heartbeatRef.current = setInterval(() => {
      if (!streamActiveRef.current) return;

      const timeSinceLastActivity = Date.now() - lastActivityRef.current;
      // If no activity for 30 seconds, consider connection stale and trigger reconnect
      if (timeSinceLastActivity > 30000) {
        console.warn('SSE connection stale, no activity for 30s — triggering reconnect');
        // Abort the current stale connection; the retry logic in attemptFetch will handle reconnection
        reconnectHandlerRef.current?.();
      }
    }, 10000); // Check every 10 seconds
  }, []);

  const stopHeartbeat = useCallback(() => {
    if (heartbeatRef.current) {
      clearInterval(heartbeatRef.current);
      heartbeatRef.current = null;
    }
  }, []);

  // Cleanup on unmount
  useEffect(() => {
    return () => {
      stopHeartbeat();
      abortRef.current?.abort();
    };
  }, [stopHeartbeat]);

  const startStream = useCallback(
    async (conversationId: string, content: string, handlers: SSEEventHandler) => {
      setIsStreaming(true);
      setConnectionState('connecting');
      setReconnectCount(0);
      streamActiveRef.current = true;

      const controller = new AbortController();
      abortRef.current = controller;

      let retries = 0;
      const maxRetries = 3;
      let responseStarted = false;
      // Track if heartbeat triggered a reconnect request
      let heartbeatReconnect = false;

      // Register heartbeat reconnect handler
      reconnectHandlerRef.current = () => {
        heartbeatReconnect = true;
        controller.abort();
      };

      const attemptFetch = async (): Promise<void> => {
        try {
          const response = await fetch(
            `/api/v1/conversations/${conversationId}/messages:stream`,
            {
              method: 'POST',
              headers: {
                'Content-Type': 'application/json',
                Authorization: getAuthToken(),
                Accept: 'text/event-stream',
              },
              body: JSON.stringify({ content }),
              signal: controller.signal,
            }
          );

          if (!response.ok) {
            let errorMsg = `HTTP ${response.status}`;
            try {
              const errBody = await response.json();
              errorMsg = errBody.message || errBody.code || errorMsg;
            } catch {
              /* ignore parse errors */
            }
            setConnectionState('error');
            handlers.onError?.({ code: 'HTTP_ERROR', message: errorMsg });
            return;
          }

          responseStarted = true;
          heartbeatReconnect = false;
          setConnectionState('connected');
          startHeartbeat();

          const reader = response.body!.getReader();
          const decoder = new TextDecoder();
          let buffer = '';

          while (true) {
            const { done, value } = await reader.read();
            if (done) break;

            // Update last activity timestamp
            lastActivityRef.current = Date.now();

            buffer += decoder.decode(value, { stream: true });

            // Split on \n\n (SSE frame delimiter)
            const frames = buffer.split('\n\n');
            // Last element might be incomplete — keep in buffer
            buffer = frames.pop() || '';

            for (const frame of frames) {
              if (!frame.trim()) continue;

              let eventType = '';
              const lines = frame.split('\n');

              for (const line of lines) {
                if (line.startsWith('event: ')) {
                  eventType = line.slice(7).trim();
                } else if (line.startsWith('data: ')) {
                  const jsonStr = line.slice(6);
                  try {
                    const data = JSON.parse(jsonStr);

                    switch (eventType) {
                      case 'status':
                        handlers.onStatus?.(data as StatusEvent);
                        break;
                      case 'evidence':
                        handlers.onEvidence?.(data as EvidenceEvent);
                        break;
                      case 'delta':
                        handlers.onDelta?.(data.content ?? '');
                        break;
                      case 'done':
                        setConnectionState('idle');
                        streamActiveRef.current = false;
                        handlers.onDone?.(data as DoneEvent);
                        return; // Stream complete
                      case 'error':
                        setConnectionState('error');
                        streamActiveRef.current = false;
                        handlers.onError?.(data as SSEErrorEvent);
                        return;
                    }
                  } catch {
                    // Skip malformed JSON — log in dev only
                    if (import.meta.env.DEV) {
                      console.warn('SSE JSON parse error:', jsonStr);
                    }
                  }
                }
              }
            }
          }

          // Stream ended normally
          setConnectionState('idle');
          streamActiveRef.current = false;
        } catch (err) {
          if ((err as Error).name === 'AbortError') {
            // If heartbeat triggered this abort, attempt reconnect
            if (heartbeatReconnect && streamActiveRef.current) {
              heartbeatReconnect = false;
              stopHeartbeat();

              if (retries < maxRetries) {
                retries++;
                setReconnectCount(retries);
                setConnectionState('reconnecting');

                if (import.meta.env.DEV) {
                  console.warn(`SSE heartbeat reconnect ${retries}/${maxRetries}`);
                }

                const delay = Math.pow(2, retries - 1) * 1000;
                await new Promise((r) => setTimeout(r, delay));

                if (!controller.signal.aborted) {
                  responseStarted = false;
                  return attemptFetch();
                }
              }

              setConnectionState('error');
              streamActiveRef.current = false;
              handlers.onError?.({
                code: 'HEARTBEAT_TIMEOUT',
                message: 'Connection lost — no data received for 30s',
              });
              return;
            }

            setConnectionState('disconnected');
            streamActiveRef.current = false;
            return; // Silent — intentional abort
          }

          // Auto-retry on network errors
          if (!responseStarted && retries < maxRetries) {
            retries++;
            setReconnectCount(retries);
            setConnectionState('reconnecting');

            if (import.meta.env.DEV) {
              console.warn(`SSE retry ${retries}/${maxRetries}`);
            }

            // Exponential backoff: 1s, 2s, 4s
            const delay = Math.pow(2, retries - 1) * 1000;
            await new Promise((r) => setTimeout(r, delay));

            // Check if still not aborted
            if (!controller.signal.aborted) {
              return attemptFetch();
            }
          }

          setConnectionState('error');
          streamActiveRef.current = false;
          handlers.onError?.({
            code: 'NETWORK_ERROR',
            message: (err as Error).message || 'Connection failed',
          });
        }
      };

      try {
        await attemptFetch();
      } finally {
        stopHeartbeat();
        reconnectHandlerRef.current = null;
        setIsStreaming(false);
        // Use ref to avoid stale closure
        const currentState = connectionStateRef.current;
        if (currentState !== 'idle' && currentState !== 'error') {
          setConnectionState('disconnected');
        }
      }
    },
    [startHeartbeat, stopHeartbeat]
  );

  const abort = useCallback(() => {
    streamActiveRef.current = false;
    stopHeartbeat();
    reconnectHandlerRef.current = null;
    abortRef.current?.abort();
    setConnectionState('disconnected');
  }, [stopHeartbeat]);

  return { startStream, abort, isStreaming, connectionState, reconnectCount };
}
