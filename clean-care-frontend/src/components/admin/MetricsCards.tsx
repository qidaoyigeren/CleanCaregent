import { useState, useEffect } from 'react';
import { getAgentMetrics } from '../../api/admin';
import type { AgentMetricsSnapshot } from '../../types/eval';
import LoadingSpinner from '../ui/LoadingSpinner';
import ErrorMessage from '../ui/ErrorMessage';

export default function MetricsCards() {
  const [metrics, setMetrics] = useState<AgentMetricsSnapshot | null>(null);
  const [error, setError] = useState<string | null>(null);

  const fetchMetrics = () => {
    getAgentMetrics()
      .then(setMetrics)
      .catch((err) => setError(err.message || 'Failed to load metrics'));
  };

  useEffect(() => {
    fetchMetrics();
    const interval = setInterval(fetchMetrics, 30000);
    return () => clearInterval(interval);
  }, []);

  if (error) return <ErrorMessage message={error} onRetry={fetchMetrics} />;
  if (!metrics) return <LoadingSpinner text="Loading metrics..." />;

  const cards = [
    { label: 'Total Requests', value: metrics.total_requests.toLocaleString(), unit: '' },
    { label: 'Success Rate', value: (metrics.success_rate * 100).toFixed(1), unit: '%' },
    { label: 'P95 Latency', value: metrics.p95_latency_ms, unit: 'ms' },
    {
      label: 'Avg Tokens',
      value: (metrics.avg_input_tokens + metrics.avg_output_tokens),
      unit: 't',
    },
  ];

  return (
    <div className="metrics-grid">
      {cards.map((card) => (
        <div key={card.label} className="metrics-card">
          <div className="metrics-card__label">{card.label}</div>
          <div className="metrics-card__value">
            {card.value}
            <span className="metrics-card__unit">{card.unit}</span>
          </div>
        </div>
      ))}
    </div>
  );
}
