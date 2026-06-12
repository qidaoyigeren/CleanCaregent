import { Link, useLocation } from 'react-router-dom';
import MetricsCards from '../components/admin/MetricsCards';

export default function AdminDashboard() {
  const location = useLocation();

  const isActive = (path: string) => location.pathname === path;

  return (
    <div className="admin-page">
      <h1 className="admin-page__title">Admin Dashboard</h1>

      <nav className="admin-nav">
        <Link
          to="/admin"
          className={`admin-nav__link ${isActive('/admin') ? 'admin-nav__link--active' : ''}`}
        >
          Overview
        </Link>
        <Link
          to="/admin/kb"
          className={`admin-nav__link ${isActive('/admin/kb') ? 'admin-nav__link--active' : ''}`}
        >
          Knowledge Base
        </Link>
        <Link
          to="/admin/eval"
          className={`admin-nav__link ${isActive('/admin/eval') ? 'admin-nav__link--active' : ''}`}
        >
          Evaluation
        </Link>
        <Link
          to="/admin/prompts"
          className={`admin-nav__link ${isActive('/admin/prompts') ? 'admin-nav__link--active' : ''}`}
        >
          Prompts
        </Link>
      </nav>

      <h3 className="section-header">Agent Metrics</h3>
      <MetricsCards />

      <div style={{ marginTop: 'var(--space-8)', display: 'flex', gap: 'var(--space-4)' }}>
        <Link
          to="/admin/kb"
          style={{
            padding: 'var(--space-4) var(--space-6)',
            background: 'var(--color-surface)',
            border: '1px solid var(--color-border)',
            borderRadius: 'var(--radius-md)',
            flex: 1,
            textDecoration: 'none',
            color: 'var(--color-text)',
          }}
        >
          <div style={{ fontWeight: 600, marginBottom: 'var(--space-1)' }}>{'📚'} Knowledge Base</div>
          <div style={{ fontSize: 'var(--text-sm)', color: 'var(--color-text-secondary)' }}>
            Upload documents and test retrieval
          </div>
        </Link>

        <Link
          to="/admin/eval"
          style={{
            padding: 'var(--space-4) var(--space-6)',
            background: 'var(--color-surface)',
            border: '1px solid var(--color-border)',
            borderRadius: 'var(--radius-md)',
            flex: 1,
            textDecoration: 'none',
            color: 'var(--color-text)',
          }}
        >
          <div style={{ fontWeight: 600, marginBottom: 'var(--space-1)' }}>{'🧪'} Evaluation</div>
          <div style={{ fontSize: 'var(--text-sm)', color: 'var(--color-text-secondary)' }}>
            Run and view evaluation results
          </div>
        </Link>
      </div>
    </div>
  );
}
