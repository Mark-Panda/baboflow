import { create } from 'zustand';
import { authApi, CurrentUser } from '@/api/auth';

interface AuthState {
  user: CurrentUser | null;
  loaded: boolean;
  fetchMe: () => Promise<CurrentUser | null>;
  setUser: (u: CurrentUser | null) => void;
  logout: () => Promise<void>;
}

export const useAuthStore = create<AuthState>((set) => ({
  user: null,
  loaded: false,
  fetchMe: async () => {
    try {
      const u = await authApi.me();
      set({ user: u, loaded: true });
      return u;
    } catch {
      set({ user: null, loaded: true });
      return null;
    }
  },
  setUser: (u) => set({ user: u }),
  logout: async () => {
    try {
      await authApi.logout();
    } finally {
      set({ user: null });
    }
  },
}));
