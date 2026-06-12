import type { PipelineState } from '../../hooks/usePipeline';

interface StatusBarProps {
  pipeline: PipelineState;
  isStreaming: boolean;
}

const STAGE_LABELS: Record<string, string> = {
  intent: 'Analyzing intent...',
  rewrite: 'Rewriting query...',
  plan: 'Planning execution...',
  retrieve: 'Retrieving knowledge...',
  tools: 'Calling tools...',
  reflect: 'Reflecting on results...',
  generate: 'Generating answer...',
};

export default function StatusBar({ pipeline, isStreaming }: StatusBarProps) {
  // Check if any step failed
  const failedStep = pipeline.steps.find((s) => s.status === 'failed');

  if (failedStep) {
    return (
      <div className="status-bar" style={{ background: 'var(--color-error-light)', borderColor: '#fecaca' }}>
        <span>{'✗'}</span>
        <span>Failed at: {failedStep.name}</span>
      </div>
    );
  }

  if (pipeline.isComplete) {
    return (
      <div className="status-bar" style={{ background: 'var(--color-success-light)', borderColor: '#bbf7d0' }}>
        <span>{'✓'}</span>
        <span>Complete</span>
      </div>
    );
  }

  if (!isStreaming) return null;

  const runningStep = pipeline.steps.find((s) => s.status === 'running');
  const label = runningStep ? STAGE_LABELS[runningStep.id] || 'Processing...' : 'Processing...';

  return (
    <div className="status-bar">
      <div className="status-bar__dot" />
      <span>{label}</span>
    </div>
  );
}

// Re-export for other files
export type { PipelineState };
