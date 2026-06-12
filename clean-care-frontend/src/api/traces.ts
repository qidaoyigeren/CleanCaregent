import { apiGet } from './client';
import type { AgentTraceRecord } from '../types/trace';

export function getTrace(traceId: string): Promise<AgentTraceRecord> {
  return apiGet<AgentTraceRecord>(`/admin/traces/${traceId}`);
}
