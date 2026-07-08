import type { ReactNode } from 'react';

export function Card({ children, className = '' }: { children: ReactNode; className?: string }) {
  return <section className={`card ${className}`.trim()}>{children}</section>;
}

export function CardBody({ children }: { children: ReactNode }) {
  return <div className="card-body">{children}</div>;
}

export function MetricCard({ label, value, caption }: { label: ReactNode; value: ReactNode; caption?: ReactNode }) {
  return (
    <Card className="metric-card">
      <CardBody>
        <div className="metric-label">{label}</div>
        <div className="metric-value">{value}</div>
        {caption ? <div className="metric-caption">{caption}</div> : null}
      </CardBody>
    </Card>
  );
}
