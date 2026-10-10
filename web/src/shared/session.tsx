import { createContext, useCallback, useContext, useEffect, useState } from 'react';
import { auth, setupApi } from '../api/admin';
import { ApiError, setCsrfToken } from '../api/client';
import type { Permission, Session, SetupInput } from '../api/types';

interface SessionState {
  /** undefined while loading, null when signed out. */
  session: Session | null | undefined;
  /** undefined while loading; true until the first restaurant exists. */
  setupRequired: boolean | undefined;
  completeSetup: (input: SetupInput) => Promise<void>;
  login: (email: string, password: string) => Promise<void>;
  logout: () => Promise<void>;
  switchTenant: (tenantId: string) => Promise<void>;
  can: (p: Permission) => boolean;
}

const Ctx = createContext<SessionState | null>(null);

export function SessionProvider({ children }: { children: React.ReactNode }) {
  const [session, setSession] = useState<Session | null | undefined>(undefined);
  const [setupRequired, setSetupRequired] = useState<boolean | undefined>(undefined);

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
    // A failed check must not lock anyone out: sign-in still works.
    setupApi
      .status()
      .then((s) => setSetupRequired(s.required))
      .catch(() => setSetupRequired(false));
  }, [apply]);

  const value: SessionState = {
    session,
    setupRequired,
    completeSetup: async (input) => {
      apply(await setupApi.complete(input));
      setSetupRequired(false);
    },
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
