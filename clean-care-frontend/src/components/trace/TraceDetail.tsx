import type { AgentTraceRecord, ToolCall } from '../../types/trace';
import type { PipelineStep } from '../../types/pipeline';
import PipelineNode from '../pipeline/PipelineNode';
import EmptyState from '../ui/EmptyState';

interface TraceDetailProps {
  trace: AgentTraceRecord;
}

function ToolCallCard({ call }: { call: ToolCall }) {
  return (
    <div className="tool-call-card">
      <div className="tool-call-card__header">
        <span className="tool-call-card__name">{call.tool_name}</span>
        <span className="tool-call-card__duration">{call.latency_ms}ms</span>
        <span style={{ fontSize: 'var(--text-xs)', color: call.status === 'success' ? 'var(--color-success)' : 'var(--color-error)' }}>
          {call.status === 'success' ? '✓' : '✗'} {call.status}
        </span>
      </div>

      <div className="tool-call-card__section">
        <div className="tool-call-card__section-label">Input</div>
        <pre className="tool-call-card__json">
          {JSON.stringify(call.arguments, null, 2)}
        </pre>
      </div>

      <div className="tool-call-card__section">
        <div className="tool-call-card__section-label">Output</div>
        <pre className="tool-call-card__json">
          {typeof call.result_summary === 'string'
            ? call.result_summary
            : JSON.stringify(call.result_summary, null, 2)}
        </pre>
      </div>

      {call.error_code && (
        <div style={{ fontSize: 'var(--text-xs)', color: 'var(--color-error)' }}>
          Error: {call.error_code}
        </div>
      )}
    </div>
  );
}

function buildPipelineSteps(trace: AgentTraceRecord): PipelineStep[] {
  return trace.steps.map((ts) => ({
    id: ts.type as PipelineStep['id'],
    name: ts.type.replace(/_/g, ' ').replace(/\b\w/g, (c) => c.toUpperCase()),
    status: ts.status,
    durationMs: ts.duration_ms,
    detail: ts.metadata
      ? Object.entries(ts.metadata)
          .filter(([, v]) => v !== null && v !== undefined)
          .map(([k, v]) => `${k}: ${typeof v === 'object' ? JSON.stringify(v) : v}`)
          .join(' | ')
      : undefined,
  }));
}

export default function TraceDetail({ trace }: TraceDetailProps) {
  const pipelineSteps = buildPipelineSteps(trace);
  const totalTokens = trace.input_tokens + trace.output_tokens;

  return (
    <div>
      {/* Overview Cards */}
      <div className="trace-overview">
        <div className="trace-overview__card">
          <div className="trace-overview__label">Intent</div>
          <div className="trace-overview__value" style={{ fontSize: 'var(--text-sm)', fontFamily: 'inherit' }}>
            {trace.intent || '—'}
          </div>
        </div>
        <div className="trace-overview__card">
          <div className="trace-overview__label">Mode</div>
          <div className="trace-overview__value" style={{ fontSize: 'var(--text-sm)', fontFamily: 'inherit' }}>
            {trace.route_mode || trace.plan?.mode || '—'}
          </div>
        </div>
        <div className="trace-overview__card">
          <div className="trace-overview__label">Latency</div>
          <div className="trace-overview__value">{trace.latency_ms}ms</div>
        </div>
        <div className="trace-overview__card">
          <div className="trace-overview__label">Tokens</div>
          <div className="trace-overview__value">
            {trace.input_tokens}/{trace.output_tokens}
            <span className="metrics-card__unit">{totalTokens}t</span>
          </div>
        </div>
        <div className="trace-overview__card">
          <div className="trace-overview__label">Evidence</div>
          <div className="trace-overview__value">{trace.evidence_ids?.length || 0}</div>
        </div>
      </div>

      {/* Steps Timeline */}
      <h3 className="section-header">Execution Steps</h3>
      {pipelineSteps.length > 0 ? (
        <div className="timeline" style={{ marginBottom: 'var(--space-6)' }}>
          {pipelineSteps.map((step, i) => (
            <PipelineNode
              key={`${step.id}_${i}`}
              step={step}
              isLast={i === pipelineSteps.length - 1}
            />
          ))}
        </div>
      ) : (
        <EmptyState icon="📋" message="No steps recorded" />
      )}

      {/* Tool Calls */}
      {trace.tool_calls && trace.tool_calls.length > 0 && (
        <>
          <h3 className="section-header">Tool Calls</h3>
          {trace.tool_calls.map((call) => (
            <ToolCallCard key={call.call_id} call={call} />
          ))}
        </>
      )}

      {/* Evidence List */}
      {trace.evidence_ids && trace.evidence_ids.length > 0 && (
        <>
          <h3 className="section-header" style={{ marginTop: 'var(--space-6)' }}>
            Evidence ({trace.evidence_ids.length})
          </h3>
          <div className="evidence-list">
            {trace.evidence_ids.map((id) => (
              <div key={id} className="evidence-list__item">
                <div className="evidence-list__header">
                  <span className="evidence-list__id">[{id}]</span>
                </div>
              </div>
            ))}
          </div>
        </>
      )}
    </div>
  );
}
