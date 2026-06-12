import { Outlet } from 'react-router-dom';
import { Component, type ReactNode } from 'react';
import Header from './Header';

/** Simple error boundary to catch rendering errors per page */
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
            Page render error: {this.state.errorMessage}
          </span>
          <button
            className="error-message__retry"
            onClick={() => this.setState({ hasError: false, errorMessage: '' })}
          >
            Retry
          </button>
        </div>
      );
    }
    return this.props.children;
  }
}

export default function Layout() {
  return (
    <div className="app-layout">
      <Header />
      <div className="app-main">
        <ErrorBoundary>
          <Outlet />
        </ErrorBoundary>
      </div>
    </div>
  );
}
