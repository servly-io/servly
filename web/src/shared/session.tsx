import { createContext, useCallback, useContext, useEffect, useState } from 'react';
import { auth } from '../api/admin';
import { ApiError, setCsrfToken } from '../api/client';
import type { Permission, Session } from '../api/types';

interface SessionState {
  /** undefined while loading, null when signed out. */
  session: Session | null | undefined;
  login: (email: string, password: string) => Promise<void>;
  logout: () => Promise<void>;
  switchTenant: (tenantId: string) => Promise<void>;
  can: (p: Permission) => boolean;
}

const Ctx = createContext<SessionState | null>(null);

export function SessionProvider({ children }: { children: React.ReactNode }) {
  const [session, setSession] = useState<Session | null | undefined>(undefined);

  const apply = useCallback((s: Session | null) => {
    setCsrfToken(s?.csrf_token ?? null);
    setSession(s);
  }, []);

  useEffect(() => {
    auth
      .session()
      .then(apply)
      .catch((err: unknown) => {
        if (err instanceof ApiError && err.status === 401) apply(null);
        else apply(null);
      });
  }, [apply]);

  const value: SessionState = {
    session,
    login: async (email, password) => apply(await auth.login(email, password)),
    logout: async () => {
      try {
        await auth.logout();
      } finally {
        apply(null);
      }
    },
    switchTenant: async (id) => apply(await auth.switchTenant(id)),
    can: (p) => !!session?.active_tenant?.permissions.includes(p),
  };
  return <Ctx.Provider value={value}>{children}</Ctx.Provider>;
}

export function useSession(): SessionState {
  const s = useContext(Ctx);
  if (!s) throw new Error('useSession outside SessionProvider');
  return s;
}
