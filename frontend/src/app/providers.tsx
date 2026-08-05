import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { useState, type ReactNode } from 'react';
import '../shared/i18n';
import { AuthProvider } from '../shared/auth/AuthProvider';

export function AppProviders({ children }: { children: ReactNode }) {
  const [queryClient] = useState(() => new QueryClient({
    defaultOptions: {
      queries: {
        retry: (failureCount, error) => {
          const status = typeof error === 'object' && error && 'status' in error ? Number(error.status) : 0;
          if (status === 401 || status === 403 || status === 404) return false;
          return failureCount < 2;
        },
        refetchOnWindowFocus: false,
      },
    },
  }));

  return (
    <QueryClientProvider client={queryClient}>
      <AuthProvider>
        {children}
      </AuthProvider>
    </QueryClientProvider>
  );
}
