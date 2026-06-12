export type StepStatus = 'pending' | 'running' | 'success' | 'failed' | 'degraded';

export type PipelineStageId =
  | 'intent'
  | 'rewrite'
  | 'plan'
  | 'retrieve'
  | 'tools'
  | 'reflect'
  | 'generate';

export interface PipelineStep {
  id: PipelineStageId;
  name: string;
  status: StepStatus;
  durationMs?: number;
  detail?: string;
  subItems?: PipelineSubItem[];
  metadata?: Record<string, unknown>;
}

export interface PipelineSubItem {
  id: string;
  label: string;
  content: string;
  kind?: 'evidence' | 'tool_call' | 'metadata';
}

/** Ordered pipeline stages */
export const PIPELINE_STAGES: { id: PipelineStageId; name: string }[] = [
  { id: 'intent', name: '意图识别' },
  { id: 'rewrite', name: '查询改写' },
  { id: 'plan', name: '计划生成' },
  { id: 'retrieve', name: '知识检索' },
  { id: 'tools', name: '工具调用' },
  { id: 'reflect', name: '反思质检' },
  { id: 'generate', name: '生成回答' },
];

/** Map backend SSE stage strings to pipeline stage IDs */
export const STAGE_MAP: Record<string, PipelineStageId> = {
  bootstrap: 'intent',
  planned: 'plan',
  plan: 'plan',
  rewrite: 'rewrite',
  rewritten: 'rewrite',
  retrieving: 'retrieve',
  retrieve: 'retrieve',
  calling_tool: 'tools',
  call_tool: 'tools',
  run_skill: 'tools',
  reflecting: 'reflect',
  reflect: 'reflect',
  generating: 'generate',
  generate: 'generate',
  finish: 'generate',
};
