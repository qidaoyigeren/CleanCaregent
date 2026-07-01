import { useCallback, useEffect, useMemo, useState, type FormEvent } from 'react';
import { Link } from 'react-router-dom';
import {
  activatePrompt,
  comparePrompts,
  getPrompts,
  type PromptTemplateSummary,
} from '../api/admin';
import ErrorMessage from '../components/ui/ErrorMessage';
import LoadingSpinner from '../components/ui/LoadingSpinner';

export default function PromptPage() {
  const [items, setItems] = useState<PromptTemplateSummary[]>([]);
  const [selected, setSelected] = useState('system');
  const [versionA, setVersionA] = useState('v3');
  const [versionB, setVersionB] = useState('v3');
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [result, setResult] = useState<Record<string, unknown> | null>(null);

  const load = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const response = await getPrompts();
      setItems(response.items);
      setSelected((currentScenario) =>
        response.items.some((item) => item.scenario === currentScenario)
          ? currentScenario
          : response.items[0]?.scenario || 'system'
      );
    } catch (err) {
      setError((err as Error).message || 'Failed to load prompts');
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void load();
  }, [load]);

  const current = useMemo(
    () => items.find((item) => item.scenario === selected),
    [items, selected]
  );

  useEffect(() => {
    if (!current) return;
    setVersionA(current.active_version);
    setVersionB(current.versions[0] || current.active_version);
  }, [current]);

  const handleCompare = async (event: FormEvent) => {
    event.preventDefault();
    setError(null);
    try {
      setResult(await comparePrompts({
        prompt_scenario: selected,
        version_a: versionA,
        version_b: versionB,
      }));
    } catch (err) {
      setError((err as Error).message || 'Prompt comparison failed');
    }
  };

  const handleActivate = async () => {
    setError(null);
    try {
      await activatePrompt(selected, versionB);
      await load();
    } catch (err) {
      setError((err as Error).message || 'Prompt activation failed');
    }
  };

  return (
    <div className="admin-page">
      <nav className="admin-nav">
        <Link to="/admin" className="admin-nav__link">Overview</Link>
        <Link to="/admin/kb" className="admin-nav__link">Knowledge Base</Link>
        <Link to="/admin/eval" className="admin-nav__link">Evaluation</Link>
        <Link to="/admin/prompts" className="admin-nav__link admin-nav__link--active">Prompts</Link>
      </nav>
      <h1 className="admin-page__title">Prompt Management</h1>

      {error && <ErrorMessage message={error} onRetry={load} />}
      {loading && <LoadingSpinner text="Loading prompts..." />}
      {!loading && current && (
        <>
          <form className="eval-form" onSubmit={handleCompare}>
            <div className="eval-form__row">
              <label className="eval-form__field">
                <span className="eval-form__label">Scenario</span>
                <select className="eval-form__input" value={selected} onChange={(event) => setSelected(event.target.value)}>
                  {items.map((item) => <option key={item.scenario}>{item.scenario}</option>)}
                </select>
              </label>
              <label className="eval-form__field">
                <span className="eval-form__label">Version A</span>
                <select className="eval-form__input" value={versionA} onChange={(event) => setVersionA(event.target.value)}>
                  {current.versions.map((version) => <option key={version}>{version}</option>)}
                </select>
              </label>
              <label className="eval-form__field">
                <span className="eval-form__label">Version B</span>
                <select className="eval-form__input" value={versionB} onChange={(event) => setVersionB(event.target.value)}>
                  {current.versions.map((version) => <option key={version}>{version}</option>)}
                </select>
              </label>
              <button className="eval-form__submit" type="submit">Compare</button>
              <button className="eval-form__submit" type="button" onClick={handleActivate}>Activate B</button>
            </div>
          </form>
          <p>Active version: <strong>{current.active_version}</strong></p>
          <h3 className="section-header">System Prompt Preview</h3>
          <pre style={{ whiteSpace: 'pre-wrap', maxHeight: 520, overflow: 'auto', padding: 16, background: 'var(--color-surface)', border: '1px solid var(--color-border)', borderRadius: 8 }}>
            {current.system}
          </pre>
          {result && (
            <>
              <h3 className="section-header">Comparison Result</h3>
              <pre style={{ whiteSpace: 'pre-wrap' }}>{JSON.stringify(result, null, 2)}</pre>
            </>
          )}
        </>
      )}
    </div>
  );
}
