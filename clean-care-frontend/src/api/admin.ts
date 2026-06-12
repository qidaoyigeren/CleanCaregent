import { apiGet, apiPost } from './client';
import type { AgentMetricsSnapshot, EvalRun } from '../types/eval';

export function getAgentMetrics(): Promise<AgentMetricsSnapshot> {
  return apiGet<AgentMetricsSnapshot>('/admin/metrics/agent');
}

export function runEval(params: {
  dataset_version: string;
  system_version?: string;
  max_cases?: number;
}): Promise<{ run_no: string }> {
  return apiPost<{ run_no: string }>('/admin/eval/runs', params);
}

export function getEvalRun(
  runNo: string,
  includeFailures: boolean = false
): Promise<EvalRun> {
  let path = `/admin/eval/runs/${runNo}`;
  if (includeFailures) path += '?include_failures=true';
  return apiGet<EvalRun>(path);
}

export interface PromptTemplateSummary {
  scenario: string;
  active_version: string;
  versions: string[];
  system: string;
  user: string;
}

export function getPrompts(): Promise<{ items: PromptTemplateSummary[] }> {
  return apiGet<{ items: PromptTemplateSummary[] }>('/admin/prompts');
}

export function activatePrompt(
  scenario: string,
  version: string
): Promise<{ scenario: string; version: string }> {
  return apiPost<{ scenario: string; version: string }>(
    `/admin/prompts/${encodeURIComponent(scenario)}/activate`,
    { version }
  );
}

export function comparePrompts(params: {
  prompt_scenario: string;
  version_a: string;
  version_b: string;
  eval_case_ids?: string[];
}): Promise<Record<string, unknown>> {
  return apiPost<Record<string, unknown>>('/admin/prompts/eval', params);
}
