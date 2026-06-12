import { useParams, useNavigate } from 'react-router-dom';
import useTrace from '../hooks/useTrace';
import TraceDetail from '../components/trace/TraceDetail';
import LoadingSpinner from '../components/ui/LoadingSpinner';
import ErrorMessage from '../components/ui/ErrorMessage';
import EmptyState from '../components/ui/EmptyState';

export default function TraceDetailPage() {
  const { traceId } = useParams();
  const navigate = useNavigate();
  const { trace, isLoading, error, refetch } = useTrace(traceId);

  return (
    <div className="trace-page">
      <button className="trace-page__back" onClick={() => navigate(-1)}>
        {'← Back'}
      </button>

      {error && <ErrorMessage message={error} onRetry={refetch} />}

      {isLoading && <LoadingSpinner text="Loading trace..." />}

      {!isLoading && !error && !trace && (
        <EmptyState icon="🔍" message="Trace not found" />
      )}

      {trace && (
        <>
          <div className="trace-page__header">
            <h1 className="trace-page__title">Trace: {trace.trace_id}</h1>
            <span className={`trace-page__status trace-page__status--${trace.status === 'success' ? 'success' : 'failed'}`}>
              {trace.status}
            </span>
          </div>
          <TraceDetail trace={trace} />
        </>
      )}
    </div>
  );
}
