export interface EvalRun {
  run_no: string;
  dataset_version: string;
  system_version?: string;
  status: 'running' | 'completed' | 'failed';
  total_cases: number;
  passed_cases?: number;
  avg_score?: number;
  created_at: string;
  finished_at?: string;
}

export interface CaseResult {
  case_id: string;
  question: string;
  expected_intent?: string;
  actual_intent?: string;
  score: number;
  passed: boolean;
  failure_reason?: string;
}

export interface MetricResult {
  intent_accuracy: number;
  answer_faithfulness: number;
  answer_relevance: number;
  tool_call_accuracy: number;
  overall_score: number;
}

export interface AgentMetricsSnapshot {
  request_count: number;
  failure_count: number;
  prompt_tokens: number;
  completion_tokens: number;
  total_tokens: number;
  p95_latency_ms: number;
  latency_samples: number;
  cost_usd: number;
}
