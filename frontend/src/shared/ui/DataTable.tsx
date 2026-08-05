import type { ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import { Card } from './Card';
import { EmptyState } from './State';

export type DataColumn<T> = {
  key: string;
  header: ReactNode;
  render: (row: T) => ReactNode;
  priority?: 'high' | 'medium' | 'low';
};

export function MobileRecordList<T>({ rows, columns }: { rows: T[]; columns: DataColumn<T>[] }) {
  const { t } = useTranslation();
  if (!rows.length) return <EmptyState />;
  return (
    <div className="record-list" aria-label={t('common.mobileRecords')}>
      {rows.map((row, index) => (
        <article className="record-card" key={index}>
          {columns.map((column) => (
            <div className="record-row" key={column.key}>
              <span className="record-label">{column.header}</span>
              <span>{column.render(row)}</span>
            </div>
          ))}
        </article>
      ))}
    </div>
  );
}

export function DataTable<T>({ rows, columns, title, tools }: { rows: T[]; columns: DataColumn<T>[]; title?: ReactNode; tools?: ReactNode }) {
  return (
    <Card className="data-table-card">
      {title || tools ? (
        <div className="page-header card-body">
          <h2 className="card-title">{title}</h2>
          {tools ? <div className="toolbar">{tools}</div> : null}
        </div>
      ) : null}
      {rows.length ? (
        <>
          <div className="data-table-wrap">
            <table className="data-table">
              <thead>
                <tr>
                  {columns.map((column) => <th key={column.key}>{column.header}</th>)}
                </tr>
              </thead>
              <tbody>
                {rows.map((row, index) => (
                  <tr key={index}>
                    {columns.map((column) => <td key={column.key}>{column.render(row)}</td>)}
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
          <MobileRecordList rows={rows} columns={columns} />
        </>
      ) : <EmptyState />}
    </Card>
  );
}
