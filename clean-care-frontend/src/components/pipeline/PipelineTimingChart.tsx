import type { PipelineStep } from '../../types/pipeline';
import { formatDuration } from '../../utils/format';

interface PipelineTimingChartProps {
  steps: PipelineStep[];
}

export default function PipelineTimingChart({ steps }: PipelineTimingChartProps) {
  const completedSteps = steps.filter((s) => s.status === 'success' || s.status === 'failed');
  const totalTime = completedSteps.reduce((sum, step) => sum + (step.durationMs || 0), 0);

  if (completedSteps.length === 0 || totalTime === 0) {
    return null;
  }

  const getStepColor = (status: string): string => {
    switch (status) {
      case 'success':
        return 'var(--color-success)';
      case 'failed':
        return 'var(--color-error)';
      case 'running':
        return 'var(--color-primary)';
      default:
        return 'var(--color-border)';
    }
  };

  return (
    <div className="timing-chart">
      <div className="timing-chart__header">
        <span className="timing-chart__title">耗时分布</span>
        <span className="timing-chart__total">总计: {formatDuration(totalTime)}</span>
      </div>
      <div className="timing-chart__bars">
        {completedSteps.map((step, index) => {
          const percentage = ((step.durationMs || 0) / totalTime) * 100;
          return (
            <div
              key={index}
              className="timing-chart__bar"
              style={{
                width: `${percentage}%`,
                backgroundColor: getStepColor(step.status),
              }}
              title={`${step.name}: ${formatDuration(step.durationMs || 0)} (${percentage.toFixed(1)}%)`}
            />
          );
        })}
      </div>
      <div className="timing-chart__legend">
        {completedSteps.slice(0, 5).map((step, index) => (
          <div key={index} className="timing-chart__legend-item">
            <span
              className="timing-chart__legend-color"
              style={{ backgroundColor: getStepColor(step.status) }}
            />
            <span className="timing-chart__legend-label">{step.name}</span>
            <span className="timing-chart__legend-value">
              {formatDuration(step.durationMs || 0)}
            </span>
          </div>
        ))}
      </div>
    </div>
  );
}
