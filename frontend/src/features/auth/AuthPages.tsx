import { useMutation, useQuery } from '@tanstack/react-query';
import { useState, type FormEvent } from 'react';
import { useTranslation } from 'react-i18next';
import { Navigate, useLocation } from 'react-router-dom';
import { getInvite } from '../../shared/api/endpoints';
import { APIError, getAPIBase, setAPIBase } from '../../shared/api/client';
import { useAuth } from '../../shared/auth/AuthProvider';
import { Button, Card, CardBody, ErrorState, FormField, FormGrid, LoadingSkeleton, TextField } from '../../shared/ui';

function inviteTokenFromSearch(search: string): string {
  return new URLSearchParams(search).get('invite_token') || '';
}

export function AuthGate() {
  const auth = useAuth();
  const location = useLocation();
  const token = inviteTokenFromSearch(location.search);

  if (auth.isLoading) {
    return <div className="auth-page"><LoadingSkeleton /></div>;
  }

  if (auth.isAuthenticated) {
    return <Navigate to="/" replace />;
  }

  return token ? <InvitePage token={token} /> : <LoginPage />;
}

export function LoginPage() {
  const { t } = useTranslation();
  const auth = useAuth();
  const [login, setLogin] = useState('');
  const [password, setPassword] = useState('');
  const [apiBase, setAPIBaseDraft] = useState(getAPIBase());
  const [error, setError] = useState('');

  async function submit(event: FormEvent) {
    event.preventDefault();
    setError('');
    try {
      await auth.login(login.trim(), password);
    } catch (err) {
      setError(err instanceof Error ? err.message : t('errors.network'));
    }
  }

  return (
    <main className="auth-page">
      <Card className="auth-panel">
        <CardBody>
          <form className="page-stack" onSubmit={submit}>
            <div>
              <h1>{t('auth.loginTitle')}</h1>
              <p>{t('auth.loginSubtitle')}</p>
            </div>
            <FormGrid>
              <FormField label={t('auth.login')} full>
                <TextField autoComplete="username" value={login} onChange={(event) => setLogin(event.currentTarget.value)} required autoFocus />
              </FormField>
              <FormField label={t('auth.password')} full>
                <TextField type="password" autoComplete="current-password" value={password} onChange={(event) => setPassword(event.currentTarget.value)} required />
              </FormField>
              <FormField label={t('auth.apiSettings')} full>
                <TextField
                  type="url"
                  placeholder={t('common.currentHost')}
                  value={apiBase}
                  onChange={(event) => setAPIBaseDraft(event.currentTarget.value)}
                  onBlur={() => setAPIBase(apiBase)}
                />
              </FormField>
            </FormGrid>
            {error ? <ErrorState body={error} /> : null}
            <div className="toolbar">
              <Button variant="primary" type="submit">{t('auth.signIn')}</Button>
              <Button type="button" onClick={() => setAPIBase(apiBase)}>{t('auth.saveApiBase')}</Button>
            </div>
          </form>
        </CardBody>
      </Card>
    </main>
  );
}

export function InvitePage({ token }: { token: string }) {
  const { t } = useTranslation();
  const auth = useAuth();
  const [password, setPassword] = useState('');
  const [error, setError] = useState('');
  const invite = useQuery({
    queryKey: ['invite', token],
    queryFn: () => getInvite(token),
    retry: false,
  });

  const mutation = useMutation({
    mutationFn: () => auth.acceptInvite(token, password),
    onSuccess: () => {
      const url = new URL(window.location.href);
      url.searchParams.delete('invite_token');
      window.history.replaceState({}, '', url.toString());
    },
  });

  async function submit(event: FormEvent) {
    event.preventDefault();
    setError('');
    try {
      await mutation.mutateAsync();
    } catch (err) {
      setError(err instanceof APIError ? err.message : t('errors.network'));
    }
  }

  return (
    <main className="auth-page">
      <Card className="auth-panel">
        <CardBody>
          <form className="page-stack" onSubmit={submit}>
            <div>
              <h1>{t('auth.inviteTitle')}</h1>
              <p>{t('auth.inviteSubtitle')}</p>
            </div>
            {invite.isLoading ? <LoadingSkeleton /> : null}
            {invite.isError ? <ErrorState body={invite.error.message} /> : null}
            {invite.data ? <pre className="code-block">{JSON.stringify(invite.data, null, 2)}</pre> : null}
            <FormField label={t('auth.newPassword')} full>
              <TextField type="password" autoComplete="new-password" value={password} onChange={(event) => setPassword(event.currentTarget.value)} required />
            </FormField>
            {error ? <ErrorState body={error} /> : null}
            <div className="toolbar">
              <Button variant="primary" type="submit">{t('auth.activate')}</Button>
              <Button type="button" onClick={() => window.location.assign('/')}>{t('auth.backToLogin')}</Button>
            </div>
          </form>
        </CardBody>
      </Card>
    </main>
  );
}
