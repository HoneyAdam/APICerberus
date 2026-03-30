import { create } from "zustand";
import { clearStoredAdminApiKey, getStoredAdminApiKey, setStoredAdminApiKey } from "@/lib/api";

export type AuthStatus = "anonymous" | "authenticated";

type AuthState = {
  initialized: boolean;
  status: AuthStatus;
  adminApiKey: string;
  hydrate: () => void;
  setAdminApiKey: (key: string) => void;
  clearSession: () => void;
};

function normalizeKey(value: string) {
  return value.trim();
}

function resolveStatus(key: string): AuthStatus {
  return key ? "authenticated" : "anonymous";
}

export const useAuthStore = create<AuthState>((set) => ({
  initialized: false,
  status: "anonymous",
  adminApiKey: "",

  hydrate: () => {
    const key = normalizeKey(getStoredAdminApiKey());
    set({
      initialized: true,
      adminApiKey: key,
      status: resolveStatus(key),
    });
  },

  setAdminApiKey: (input) => {
    const key = normalizeKey(input);
    setStoredAdminApiKey(key);
    set({
      initialized: true,
      adminApiKey: key,
      status: resolveStatus(key),
    });
  },

  clearSession: () => {
    clearStoredAdminApiKey();
    set({
      initialized: true,
      adminApiKey: "",
      status: "anonymous",
    });
  },
}));

