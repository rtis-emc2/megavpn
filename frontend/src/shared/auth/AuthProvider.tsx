import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { createContext, useContext, useMemo, type ReactNode } from 'react';
import { acceptInvite, changePassword as changePasswordRequest, getSession, login as loginRequest, logout as logoutRequest } from '../api/endpoints';
import type { AuthPayload } from '../api/types';

type AuthContextValue = {
  session: AuthPayload | null;
  isLoading: boolean;
  isAuthenticated: boolean;
  roles: string[];
  permissions: string[];
  login: (login: string, password: string) => Promise<void>;
  logout: () => Promise<void>;
  changePassword: (currentPassword: string, newPassword: string) => Promise<void>;
  acceptInvite: (token: string, password: string) => Promise<void>;
};

const AuthContext = createContext<AuthContextValue | null>(null);

export function AuthProvider({ children }: { children: ReactNode }) {
  const queryClient = useQueryClient();
  const sessionQuery = useQuery({
    queryKey: ['auth', 'session'],
    queryFn: getSession,
    retry: false,
    staleTime: 15_000,
  });

  const loginMutation = useMutation({
    mutationFn: ({ login, password }: { login: string; password: string }) => loginRequest(login, password),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ['auth', 'session'] });
      await queryClient.invalidateQueries();
    },
  });

  const logoutMutation = useMutation({
    mutationFn: logoutRequest,
    onSettled: async () => {
      queryClient.setQueryData(['auth', 'session'], null);
      await queryClient.invalidateQueries({ queryKey: ['auth', 'session'] });
    },
  });

  const inviteMutation = useMutation({
    mutationFn: ({ token, password }: { token: string; password: string }) => acceptInvite(token, password),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ['auth', 'session'] });
    },
  });

  const changePasswordMutation = useMutation({
    mutationFn: ({ currentPassword, newPassword }: { currentPassword: string; newPassword: string }) => changePasswordRequest(currentPassword, newPassword),
    onSuccess: () => {
      queryClient.setQueryData(['auth', 'session'], null);
    },
  });

  const session = sessionQuery.data || null;
  const value = useMemo<AuthContextValue>(() => ({
    session,
    isLoading: sessionQuery.isLoading,
    isAuthenticated: Boolean(session?.user),
    roles: session?.roles || [],
    permissions: session?.permissions || [],
    login: async (login, password) => {
      await loginMutation.mutateAsync({ login, password });
    },
    logout: async () => {
      await logoutMutation.mutateAsync();
    },
    changePassword: async (currentPassword, newPassword) => {
      await changePasswordMutation.mutateAsync({ currentPassword, newPassword });
    },
    acceptInvite: async (token, password) => {
      await inviteMutation.mutateAsync({ token, password });
    },
  }), [changePasswordMutation, inviteMutation, loginMutation, logoutMutation, session, sessionQuery.isLoading]);

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>;
}

export function useAuth() {
  const value = useContext(AuthContext);
  if (!value) throw new Error('useAuth must be used within AuthProvider');
  return value;
}
