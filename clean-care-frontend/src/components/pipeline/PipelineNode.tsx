import { useState } from 'react';
import type { PipelineStep } from '../../types/pipeline';

interface PipelineNodeProps {
  step: PipelineStep;
  isLast: boolean;
}

export default function PipelineNode({ step, isLast }: PipelineNodeProps) {
  const [expanded, setExpanded] = useState(false);

  const statusIcons: Record<string, string> = {
    pending: '○',
    running: '◉',
    success: '✓',
    failed: '✗',
    degraded: '⚠',
  };

  const hasDetails = !!(
    step.detail ||
    (step.subItems && step.subItems.length > 0) ||
    (step.metadata && Object.keys(step.metadata).length > 0)
  );

  return (
    <div className={`timeline-node timeline-node--${step.status} ${isLast ? 'timeline-node--last' : ''}`}>
      <div className="timeline-node__indicator">
        {statusIcons[step.status] || '○'}
      </div>

      <div
        className="timeline-node__content"
        onClick={() => hasDetails && setExpanded(!expanded)}
        style={{ cursor: hasDetails ? 'pointer' : 'default' }}
      >
        <div className="timeline-node__header">
          <span className="timeline-node__name">{step.name}</span>
          {step.durationMs !== undefined && step.durationMs > 0 && (
            <span className="timeline-node__duration">{step.durationMs}ms</span>
          )}
        </div>

        {step.detail && !expanded && (
          <div className="timeline-node__detail">{step.detail}</div>
        )}

        {expanded && (
          <>
            {step.detail && (
              <div className="timeline-node__detail" style={{ marginBottom: 4 }}>
                {step.detail}
              </div>
            )}

            {step.subItems && step.subItems.length > 0 && (
              <div className="timeline-node__subitems">
                {step.subItems.map((item) => (
                  <div
                    key={item.id}
                    className="timeline-node__subitem"
                    {...(item.kind === 'evidence' ? { 'data-evidence-id': item.id } : {})}
                  >
                    <span className="timeline-node__subitem-label">{item.label}</span>
                    <span
                      className="timeline-node__subitem-content"
                      title={item.content}
                    >
                      {item.content.length > 80
                        ? item.content.slice(0, 80) + '...'
                        : item.content}
                    </span>
                  </div>
                ))}
              </div>
            )}

            {step.metadata && Object.keys(step.metadata).length > 0 && (
              <div style={{ marginTop: 4, fontSize: 'var(--text-xs)', color: 'var(--color-text-muted)' }}>
                {Object.entries(step.metadata).map(([k, v]) => (
                  <div key={k}>
                    {k}: {typeof v === 'object' ? JSON.stringify(v) : String(v)}
                  </div>
                ))}
              </div>
            )}
          </>
        )}

        {hasDetails && !expanded && (
          <div style={{ fontSize: 'var(--text-xs)', color: 'var(--color-text-muted)', marginTop: 2 }}>
            Click to expand
          </div>
        )}
      </div>
    </div>
  );
}
