import { useState, useEffect } from 'react';
import type { PipelineStep } from '../../types/pipeline';
import PipelineNode from './PipelineNode';
import EmptyState from '../ui/EmptyState';

interface PipelinePanelProps {
  steps: PipelineStep[];
  traceId: string | null;
}

export default function PipelinePanel({ steps, traceId }: PipelinePanelProps) {
  const [collapsed, setCollapsed] = useState(false);

  // Listen for evidence-click events from ChatArea
  useEffect(() => {
    const handler = (e: Event) => {
      const detail = (e as CustomEvent).detail;
      if (detail?.evidenceId) {
        setCollapsed(false);
        // Scroll to evidence in pipeline
        setTimeout(() => {
          const el = document.querySelector(`[data-evidence-id="${detail.evidenceId}"]`);
          el?.scrollIntoView({ behavior: 'smooth', block: 'center' });
        }, 100);
      }
    };
    window.addEventListener('evidence-click', handler);
    return () => window.removeEventListener('evidence-click', handler);
  }, []);

  const hasAnyActivity = steps.some((s) => s.status !== 'pending');

  if (collapsed) {
    return (
      <div className="pipeline-panel" style={{ width: 40, minWidth: 40, padding: 'var(--space-2)' }}>
        <button className="pipeline-panel__collapse" onClick={() => setCollapsed(false)} title="Expand pipeline">
          {'◀'}
        </button>
      </div>
    );
  }

  return (
    <div className="pipeline-panel">
      <div className="pipeline-panel__header">
        <div>
          <div className="pipeline-panel__title">Agent Pipeline</div>
          {traceId && <div className="pipeline-panel__trace">{traceId}</div>}
        </div>
        <button className="pipeline-panel__collapse" onClick={() => setCollapsed(true)} title="Collapse pipeline">
          {'▶'}
        </button>
      </div>

      {!hasAnyActivity ? (
        <EmptyState icon="🔍" message="Send a message to see the agent execution pipeline" />
      ) : (
        <div className="timeline">
          {steps.map((step, i) => (
            <PipelineNode
              key={step.id}
              step={step}
              isLast={i === steps.length - 1}
            />
          ))}
        </div>
      )}
    </div>
  );
}
