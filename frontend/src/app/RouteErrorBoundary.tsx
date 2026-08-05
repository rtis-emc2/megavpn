import { Component, type ErrorInfo, type ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import { Button } from '../shared/ui';

type RouteErrorBoundaryProps = {
  children: ReactNode;
};

type RouteErrorBoundaryState = {
  error: Error | null;
};

function RouteErrorFallback({ onRetry }: { onRetry: () => void }) {
  const { t } = useTranslation();
  return (
    <main className="route-error-page">
      <section className="card route-error-card" role="alert">
        <div className="card-body page-stack">
          <div>
            <h1 className="card-title">{t('common.errorTitle')}</h1>
            <p className="muted">{t('common.safeErrorBody')}</p>
          </div>
          <div className="toolbar">
            <Button type="button" variant="primary" onClick={onRetry}>
              {t('common.retry')}
            </Button>
            <Button type="button" onClick={() => window.location.reload()}>
              {t('common.reload')}
            </Button>
          </div>
        </div>
      </section>
    </main>
  );
}

export class RouteErrorBoundary extends Component<RouteErrorBoundaryProps, RouteErrorBoundaryState> {
  state: RouteErrorBoundaryState = { error: null };

  static getDerivedStateFromError(error: Error): RouteErrorBoundaryState {
    return { error };
  }

  componentDidCatch(error: Error, info: ErrorInfo) {
    window.dispatchEvent(new CustomEvent('megavpn:frontend-error', {
      detail: {
        errorType: error.name,
        componentDepth: info.componentStack?.split('\n').filter(Boolean).length || 0,
      },
    }));
  }

  private retry = () => {
    this.setState({ error: null });
  };

  render() {
    if (this.state.error) {
      return <RouteErrorFallback onRetry={this.retry} />;
    }
    return this.props.children;
  }
}
