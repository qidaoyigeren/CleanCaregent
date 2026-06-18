import { Outlet } from 'react-router-dom';
import { Component, type ReactNode, useEffect } from 'react';
import Sidebar from './Sidebar';
import ShortcutsHelp from '../ui/ShortcutsHelp';
import OfflineBanner from '../ui/OfflineBanner';
import { useAppStore } from '../../store/appStore';
import { initHljsTheme } from '../../utils/hljsTheme';

/** Error boundary to catch rendering errors per page */
export class ErrorBoundary extends Component<
  { children: ReactNode; fallback?: ReactNode },
  { hasError: boolean; errorMessage: string }
> {
  state = { hasError: false, errorMessage: '' };

  static getDerivedStateFromError(error: Error) {
    return { hasError: true, errorMessage: error.message };
  }

  render() {
    if (this.state.hasError) {
      if (this.props.fallback) return this.props.fallback;
      return (
        <div className="error-message" style={{ margin: 24 }}>
          <span>{'⚠'}</span>
          <span className="error-message__text">
            页面渲染错误: {this.state.errorMessage}
          </span>
          <button
            className="error-message__retry"
            onClick={() => this.setState({ hasError: false, errorMessage: '' })}
          >
            重试
          </button>
        </div>
      );
    }
    return this.props.children;
  }
}

export default function Layout() {
  const { theme, setTheme, setSidebarOpen } = useAppStore();

  // Initialize theme on mount
  useEffect(() => {
    setTheme(theme);
    const cleanup = initHljsTheme();
    return cleanup;
  }, []); // eslint-disable-line react-hooks/exhaustive-deps

  useEffect(() => {
    const mediaQuery = window.matchMedia('(max-width: 900px)');
    const syncSidebar = () => setSidebarOpen(!mediaQuery.matches);

    syncSidebar();
    mediaQuery.addEventListener('change', syncSidebar);
    return () => mediaQuery.removeEventListener('change', syncSidebar);
  }, [setSidebarOpen]);

  return (
    <div className="app-layout">
      <OfflineBanner />
      <Sidebar />
      <main className="app-main">
        <ErrorBoundary>
          <Outlet />
        </ErrorBoundary>
      </main>
      <ShortcutsHelp />
    </div>
  );
}
