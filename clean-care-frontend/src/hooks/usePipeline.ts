import { useReducer } from 'react';
import {
  PIPELINE_STAGES,
  STAGE_MAP,
} from '../types/pipeline';
import type {
  PipelineStep,
  PipelineStageId,
  StepStatus,
} from '../types/pipeline';
import type { AgentTraceRecord } from '../types/trace';
import type { EvidenceEvent, StatusEvent } from '../types/sse';

export interface PipelineState {
  steps: PipelineStep[];
  traceId: string | null;
  isComplete: boolean;
}

type ActionType =
  | 'RESET'
  | 'ADVANCE_STAGE'
  | 'APPEND_EVIDENCE'
  | 'MARK_COMPLETE'
  | 'ENRICH_TRACE'
  | 'MARK_FAILED';

export interface PipelineAction {
  type: ActionType;
  stage?: string;
  detail?: StatusEvent;
  evidence?: EvidenceEvent;
  trace?: AgentTraceRecord;
  error?: string;
}

function createInitialSteps(): PipelineStep[] {
  return PIPELINE_STAGES.map((s) => ({
    id: s.id,
    name: s.name,
    status: 'pending' as StepStatus,
  }));
}

function stageNameToId(stage: string): PipelineStageId | null {
  // Direct match first
  if (STAGE_MAP[stage]) return STAGE_MAP[stage];

  // Prefix match: check if stage starts with a known prefix
  const lower = stage.toLowerCase();
  for (const { id } of PIPELINE_STAGES) {
    if (lower.startsWith(id)) return id;
  }

  // Contains match as last resort (only for longer known stage IDs to avoid false positives)
  for (const { id } of PIPELINE_STAGES) {
    if (id.length >= 4 && lower.includes(id)) return id;
  }

  return null;
}

function pipelineReducer(state: PipelineState, action: PipelineAction): PipelineState {
  switch (action.type) {
    case 'RESET':
      return { steps: createInitialSteps(), traceId: null, isComplete: false };

    case 'ADVANCE_STAGE': {
      const stageId = action.stage ? stageNameToId(action.stage) : null;
      if (!stageId) return state;

      const steps = state.steps.map((step) => {
        const stageIndex = PIPELINE_STAGES.findIndex((s) => s.id === stageId);
        const stepIndex = PIPELINE_STAGES.findIndex((s) => s.id === step.id);

        if (stepIndex < stageIndex) {
          // Prior stages: mark success if not already
          return step.status === 'pending' ? { ...step, status: 'success' as StepStatus } : step;
        } else if (stepIndex === stageIndex) {
          return {
            ...step,
            status: 'running' as StepStatus,
            detail: action.detail
              ? [
                  action.detail.intent && `Intent: ${action.detail.intent}`,
                  action.detail.confidence !== undefined &&
                    `Confidence: ${(action.detail.confidence * 100).toFixed(0)}%`,
                  action.detail.mode && `Mode: ${action.detail.mode}`,
                ]
                  .filter(Boolean)
                  .join(' | ')
              : step.detail,
          };
        } else {
          return step;
        }
      });

      // Track trace_id from status events
      const traceId = action.detail?.trace_id || state.traceId;

      return { ...state, steps, traceId };
    }

    case 'APPEND_EVIDENCE': {
      if (!action.evidence) return state;

      const steps = state.steps.map((step) => {
        if (step.id === 'retrieve') {
          const existing = step.subItems || [];
          const newItem = {
            id: action.evidence!.evidence_id,
            label: `[${action.evidence!.evidence_id}]`,
            content: action.evidence!.title,
            kind: 'evidence' as const,
          };
          // Avoid duplicates
          if (existing.some((e) => e.id === newItem.id)) return step;
          return {
            ...step,
            subItems: [...existing, newItem],
          };
        }
        return step;
      });

      return { ...state, steps };
    }

    case 'MARK_COMPLETE': {
      const steps = state.steps.map((step) => ({
        ...step,
        status:
          step.status === 'running' ? ('success' as StepStatus) : step.status,
      }));
      return { ...state, steps, isComplete: true };
    }

    case 'ENRICH_TRACE': {
      if (!action.trace) return state;

      const trace = action.trace;
      const stepMap = new Map<string, typeof trace.steps[0]>();
      for (const s of trace.steps) {
        stepMap.set(s.type, s);
      }

      let runningFound = false;
      const steps = state.steps.map((step) => {
        const traceStep = stepMap.get(step.id);
        let status: StepStatus = step.status;

        if (!runningFound && step.status === 'running') {
          runningFound = true;
        }

        if (traceStep) {
          return {
            ...step,
            status: traceStep.status as StepStatus,
            durationMs: traceStep.duration_ms,
            detail: traceStep.metadata
              ? Object.entries(traceStep.metadata)
                  .filter(([, v]) => v !== null && v !== undefined)
                  .map(([k, v]) => `${k}: ${typeof v === 'object' ? JSON.stringify(v) : v}`)
                  .join(' | ') || step.detail
              : step.detail,
          };
        }

        // Mark unsent stages as success if they precede a completed stage
        if (!runningFound) {
          status = 'success';
        }

        return { ...step, status };
      });

      // Add token info to generate step
      const genSteps = steps.map((step) => {
        if (step.id === 'generate') {
          return {
            ...step,
            detail: `Input: ${trace.input_tokens}t Output: ${trace.output_tokens}t`,
          };
        }
        return step;
      });

      // If tool_calls exist, enrich the tools stage
      const toolCalls = trace.tool_calls || [];
      let finalSteps = genSteps;
      if (toolCalls.length > 0) {
        finalSteps = genSteps.map((step) => {
          if (step.id === 'tools') {
            return {
              ...step,
              status: 'success' as StepStatus,
              subItems: toolCalls.map((tc) => ({
                id: tc.call_id,
                label: tc.tool_name,
                content: `${tc.status} | ${tc.latency_ms}ms | Results: ${JSON.stringify(tc.result_summary).slice(0, 60)}`,
                kind: 'tool_call' as const,
              })),
            };
          }
          return step;
        });
      }

      return {
        ...state,
        steps: finalSteps,
        traceId: trace.trace_id,
        isComplete: true,
      };
    }

    case 'MARK_FAILED': {
      const steps = state.steps.map((step) => ({
        ...step,
        status:
          step.status === 'running' ? ('failed' as StepStatus) : step.status,
      }));
      return { ...state, steps };
    }

    default:
      return state;
  }
}

const initialState: PipelineState = {
  steps: createInitialSteps(),
  traceId: null,
  isComplete: false,
};

export default function usePipeline() {
  const [state, dispatch] = useReducer(pipelineReducer, initialState);
  return { pipeline: state, dispatch };
}
