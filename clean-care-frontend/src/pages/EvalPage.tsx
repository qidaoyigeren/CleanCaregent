import { useState, useEffect, useRef } from 'react';
import type { FormEvent } from 'react';
import { Link } from 'react-router-dom';
import { runEval, getEvalRun } from '../api/admin';
import { ApiError } from '../types/api';
import type { EvalRun } from '../types/eval';
import ErrorMessage from '../components/ui/ErrorMessage';
import LoadingSpinner from '../components/ui/LoadingSpinner';
import EmptyState from '../components/ui/EmptyState';

export default function EvalPage() {
  const [datasetVersion, setDatasetVersion] = useState('v1');
  const [maxCases, setMaxCases] = useState('100');
  const [isRunning, setIsRunning] = useState(false);
  const [runError, setRunError] = useState<string | null>(null);
  const [runs, setRuns] = useState<EvalRun[]>([]);
  const [runsLoading] = useState(false);
  const [runsError] = useState<string | null>(null);

  // Ref to track active polling interval for cleanup on unmount
  const pollRef = useRef<ReturnType<typeof setInterval> | null>(null);

  // Cleanup polling on unmount
  useEffect(() => {
    return () => {
      if (pollRef.current) clearInterval(pollRef.current);
    };
  }, []);

  const handleRunEval = async (e: FormEvent) => {
    e.preventDefault();
    setIsRunning(true);
    setRunError(null);

    try {
      const result = await runEval({
        dataset_version: datasetVersion,
        max_cases: parseInt(maxCases) || undefined,
      });
      // Add to local list
      const newRun: EvalRun = {
        run_no: result.run_no,
        dataset_version: datasetVersion,
        status: 'running',
        total_cases: parseInt(maxCases) || 0,
        created_at: new Date().toISOString(),
      };
      setRuns((prev) => [newRun, ...prev]);

      // Clear any existing poll before starting a new one
      if (pollRef.current) clearInterval(pollRef.current);

      let pollCount = 0;
      pollRef.current = setInterval(async () => {
        pollCount++;
        try {
          const updated = await getEvalRun(result.run_no, true);
          setRuns((prev) =>
            prev.map((r) => (r.run_no === result.run_no ? updated : r))
          );
          if (updated.status !== 'running' || pollCount >= 30) {
            if (pollRef.current) { clearInterval(pollRef.current); pollRef.current = null; }
          }
        } catch {
          if (pollRef.current) { clearInterval(pollRef.current); pollRef.current = null; }
        }
      }, 3000);
    } catch (err) {
      setRunError(err instanceof ApiError ? err.message : 'Failed to start evaluation');
    } finally {
      setIsRunning(false);
    }
  };

  return (
    <div className="admin-page">
      <nav className="admin-nav">
        <Link to="/admin" className="admin-nav__link">Overview</Link>
        <Link to="/admin/kb" className="admin-nav__link">Knowledge Base</Link>
        <Link to="/admin/eval" className="admin-nav__link admin-nav__link--active">Evaluation</Link>
        <Link to="/admin/prompts" className="admin-nav__link">Prompts</Link>
      </nav>

      <h1 className="admin-page__title">Evaluation</h1>

      {/* Trigger Eval */}
      <h3 className="section-header">Run Evaluation</h3>
      <form className="eval-form" onSubmit={handleRunEval}>
        <div className="eval-form__row">
          <div className="eval-form__field">
            <label className="eval-form__label">Dataset Version</label>
            <input
              className="eval-form__input"
              type="text"
              value={datasetVersion}
              onChange={(e) => setDatasetVersion(e.target.value)}
              style={{ width: 120 }}
            />
          </div>
          <div className="eval-form__field">
            <label className="eval-form__label">Max Cases</label>
            <input
              className="eval-form__input"
              type="number"
              value={maxCases}
              onChange={(e) => setMaxCases(e.target.value)}
              style={{ width: 100 }}
            />
          </div>
          <button
            type="submit"
            className="eval-form__submit"
            disabled={isRunning}
          >
            {isRunning ? 'Running...' : '▶ Run Eval'}
          </button>
        </div>
      </form>

      {runError && <ErrorMessage message={runError} onRetry={() => setRunError(null)} />}

      {/* Eval Runs List */}
      <h3 className="section-header" style={{ marginTop: 'var(--space-6)' }}>Eval Runs</h3>
      {runsError && <ErrorMessage message={runsError} />}
      {runsLoading && <LoadingSpinner text="Loading runs..." />}
      {!runsLoading && !runsError && runs.length === 0 && (
        <EmptyState icon="🧪" message="No evaluation runs yet. Trigger one above." />
      )}
      {runs.length > 0 && (
        <table className="eval-runs-table">
          <thead>
            <tr>
              <th>Run #</th>
              <th>Dataset</th>
              <th>Status</th>
              <th>Cases</th>
              <th>Passed</th>
              <th>Avg Score</th>
            </tr>
          </thead>
          <tbody>
            {runs.map((run) => (
              <tr key={run.run_no}>
                <td style={{ fontFamily: 'var(--font-mono)' }}>{run.run_no}</td>
                <td>{run.dataset_version}</td>
                <td>
                  <span
                    style={{
                      padding: '1px 8px',
                      borderRadius: 'var(--radius-full)',
                      fontSize: 'var(--text-xs)',
                      fontWeight: 600,
                      background:
                        run.status === 'completed'
                          ? 'var(--color-success-light)'
                          : run.status === 'failed'
                          ? 'var(--color-error-light)'
                          : 'var(--color-warning-light)',
                      color:
                        run.status === 'completed'
                          ? 'var(--color-success)'
                          : run.status === 'failed'
                          ? 'var(--color-error)'
                          : 'var(--color-warning)',
                    }}
                  >
                    {run.status}
                  </span>
                </td>
                <td>{run.total_cases}</td>
                <td>{run.passed_cases ?? '—'}</td>
                <td>{run.avg_score != null ? (run.avg_score * 100).toFixed(1) + '%' : '—'}</td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </div>
  );
}
