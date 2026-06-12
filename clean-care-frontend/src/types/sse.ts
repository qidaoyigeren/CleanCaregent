/** SSE event: status update from agent pipeline */
export interface StatusEvent {
  stage: string;
  mode: string;
  intent?: string;
  confidence?: number;
  trace_id: string;
}

/** SSE event: evidence chunk retrieved */
export interface EvidenceEvent {
  evidence_id: string;
  kind: 'kb_chunk' | 'tool_result';
  source_id?: string;
  title: string;
  content: string;
  metadata?: Record<string, unknown>;
}

/** SSE event: text delta for streaming answer */
export interface DeltaEvent {
  content: string;
}

/** SSE event: stream completion */
export interface DoneEvent {
  message_id: string;
  trace_id: string;
  finish_reason: string;
  mode: string;
}

/** SSE event: error */
export interface SSEErrorEvent {
  code: string;
  message: string;
}

/** Union of all SSE event types */
export type SSEEvent =
  | { type: 'status'; data: StatusEvent }
  | { type: 'evidence'; data: EvidenceEvent }
  | { type: 'delta'; data: DeltaEvent }
  | { type: 'done'; data: DoneEvent }
  | { type: 'error'; data: SSEErrorEvent };
