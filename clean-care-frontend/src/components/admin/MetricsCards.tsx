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
      .then((data) => {
        setMetrics(data);
        setError(null);
      })
      .catch((err) => setError(err.message || 'Failed to load metrics'));
  };

  useEffect(() => {
    fetchMetrics();
    const interval = setInterval(fetchMetrics, 30000);
    return () => clearInterval(interval);
  }, []);

  if (error) return <ErrorMessage message={error} onRetry={fetchMetrics} />;
  if (!metrics) return <LoadingSpinner text="Loading metrics..." />;

  const totalRequests = metrics.request_count ?? 0;
  const failureCount = metrics.failure_count ?? 0;
  const successRate =
    totalRequests > 0 ? Math.max(0, totalRequests - failureCount) / totalRequests : 0;
  const averageTokens =
    totalRequests > 0 ? Math.round((metrics.total_tokens ?? 0) / totalRequests) : 0;

  const cards = [
    { label: 'Total Requests', value: totalRequests.toLocaleString(), unit: '' },
    { label: 'Success Rate', value: (successRate * 100).toFixed(1), unit: '%' },
    { label: 'P95 Latency', value: metrics.p95_latency_ms ?? 0, unit: 'ms' },
    {
      label: 'Avg Tokens',
      value: averageTokens,
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
