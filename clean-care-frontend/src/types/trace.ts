export interface Step {
  step_id: string;
  type: 'retrieve' | 'call_tool' | 'run_skill' | 'clarify' | 'reflect' | 'finish' | 'answer_direct';
  status: 'pending' | 'running' | 'success' | 'failed';
  duration_ms: number;
  metadata: Record<string, unknown>;
}

export interface ToolCall {
  call_id: string;
  tool_name: string;
  arguments: Record<string, unknown>;
  result_summary: unknown;
  status: 'success' | 'failed' | 'timeout';
  error_code: string;
  latency_ms: number;
  created_at: string;
}

export interface AgentTraceRecord {
  trace_id: string;
  conversation_id?: string;
  intent?: string;
  route_mode?: string;
  plan?: {
    mode: string;
    steps?: unknown[];
  };
  steps: Step[];
  tool_calls: ToolCall[];
  status: string;
  evidence_ids: string[];
  input_tokens: number;
  output_tokens: number;
  latency_ms: number;
  finished_at?: string;
}
