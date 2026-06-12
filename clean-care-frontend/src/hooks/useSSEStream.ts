import { useCallback, useRef, useState } from 'react';
import type { StatusEvent, EvidenceEvent, DoneEvent, SSEErrorEvent } from '../types/sse';
import { getAuthToken } from '../auth/token';

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
}

export default function useSSEStream(): SSEStreamResult {
  const [isStreaming, setIsStreaming] = useState(false);
  const abortRef = useRef<AbortController | null>(null);

  const startStream = useCallback(
    async (conversationId: string, content: string, handlers: SSEEventHandler) => {
      setIsStreaming(true);
      const controller = new AbortController();
      abortRef.current = controller;

      let retries = 0;
      const maxRetries = 3;
      let responseStarted = false;

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
            handlers.onError?.({ code: 'HTTP_ERROR', message: errorMsg });
            return;
          }
          responseStarted = true;

          const reader = response.body!.getReader();
          const decoder = new TextDecoder();
          let buffer = '';

          while (true) {
            const { done, value } = await reader.read();
            if (done) break;

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
                        handlers.onDone?.(data as DoneEvent);
                        return; // Stream complete
                      case 'error':
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
        } catch (err) {
          if ((err as Error).name === 'AbortError') {
            return; // Silent — intentional abort
          }

          // Auto-retry on network errors
          if (!responseStarted && retries < maxRetries) {
            retries++;
            if (import.meta.env.DEV) {
              console.warn(`SSE retry ${retries}/${maxRetries}`);
            }
            await new Promise((r) => setTimeout(r, 1000 * retries));
            return attemptFetch();
          }

          handlers.onError?.({
            code: 'NETWORK_ERROR',
            message: (err as Error).message || 'Connection failed',
          });
        }
      };

      try {
        await attemptFetch();
      } finally {
        setIsStreaming(false);
      }
    },
    []
  );

  const abort = useCallback(() => {
    abortRef.current?.abort();
  }, []);

  return { startStream, abort, isStreaming };
}
