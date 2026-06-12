import { useState } from 'react';
import type { FormEvent } from 'react';
import { searchKnowledge } from '../../api/knowledge';
import type { SearchResult } from '../../api/knowledge';
import LoadingSpinner from '../ui/LoadingSpinner';
import ErrorMessage from '../ui/ErrorMessage';
import EmptyState from '../ui/EmptyState';

export default function KnowledgeSearch() {
  const [query, setQuery] = useState('');
  const [results, setResults] = useState<SearchResult[]>([]);
  const [isLoading, setIsLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [hasSearched, setHasSearched] = useState(false);

  const handleSearch = async (e: FormEvent) => {
    e.preventDefault();
    if (!query.trim()) return;

    setIsLoading(true);
    setError(null);
    setHasSearched(true);

    try {
      const data = await searchKnowledge(query.trim(), 'hybrid');
      setResults(data.results);
    } catch (err) {
      setError((err as Error).message || 'Search failed');
      setResults([]);
    } finally {
      setIsLoading(false);
    }
  };

  return (
    <div className="kb-search">
      <form className="kb-search__form" onSubmit={handleSearch}>
        <input
          className="kb-search__input"
          type="text"
          value={query}
          onChange={(e) => setQuery(e.target.value)}
          placeholder="Search knowledge base (e.g. 'T20 filter specifications')"
        />
        <button className="kb-search__btn" type="submit" disabled={isLoading}>
          {isLoading ? 'Searching...' : 'Search'}
        </button>
      </form>

      {error && <ErrorMessage message={error} onRetry={() => { setError(null); setHasSearched(false); }} />}

      {isLoading && <LoadingSpinner text="Searching..." />}

      {!isLoading && !error && hasSearched && results.length === 0 && (
        <EmptyState icon="🔍" message="No results found" />
      )}

      {results.length > 0 && (
        <div>
          <div style={{ fontSize: 'var(--text-sm)', color: 'var(--color-text-secondary)', marginBottom: 'var(--space-3)' }}>
            {results.length} result{results.length !== 1 ? 's' : ''}
          </div>
          {results.map((r) => (
            <div key={r.chunk_id} className="kb-result">
              <div className="kb-result__header">
                <span className="kb-result__score">{r.score.toFixed(3)}</span>
                <span className="evidence-list__kind">{r.doc_type || 'unknown'}</span>
                <span style={{ fontSize: 'var(--text-sm)', fontWeight: 500 }}>{r.title}</span>
              </div>
              <div className="kb-result__content">{r.content.slice(0, 300)}</div>
            </div>
          ))}
        </div>
      )}
    </div>
  );
}
