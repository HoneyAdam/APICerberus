import { create } from "zustand";
import { STORAGE_KEYS } from "@/lib/constants";

export type ThemeMode = "system" | "light" | "dark";
export type ResolvedThemeMode = "light" | "dark";

type ThemeState = {
  initialized: boolean;
  mode: ThemeMode;
  resolvedMode: ResolvedThemeMode;
  hydrate: () => void;
  setMode: (mode: ThemeMode) => void;
  toggleMode: () => void;
};

const SYSTEM_QUERY = "(prefers-color-scheme: dark)";
let mediaQueryList: MediaQueryList | null = null;
let mediaQueryCleanup: (() => void) | null = null;

function isThemeMode(value: string | null): value is ThemeMode {
  return value === "system" || value === "light" || value === "dark";
}

function readStoredMode(): ThemeMode {
  if (typeof window === "undefined") {
    return "system";
  }
  const value = window.localStorage.getItem(STORAGE_KEYS.themeMode);
  return isThemeMode(value) ? value : "system";
}

function writeStoredMode(mode: ThemeMode) {
  if (typeof window === "undefined") {
    return;
  }
  window.localStorage.setItem(STORAGE_KEYS.themeMode, mode);
}

function detectSystemMode(): ResolvedThemeMode {
  if (typeof window === "undefined") {
    return "light";
  }
  return window.matchMedia(SYSTEM_QUERY).matches ? "dark" : "light";
}

function resolveMode(mode: ThemeMode): ResolvedThemeMode {
  if (mode === "system") {
    return detectSystemMode();
  }
  return mode;
}

function applyResolvedMode(mode: ResolvedThemeMode) {
  if (typeof document === "undefined") {
    return;
  }
  const root = document.documentElement;
  root.classList.toggle("dark", mode === "dark");
  root.setAttribute("data-theme", mode);
}

function clearSystemListener() {
  if (mediaQueryCleanup) {
    mediaQueryCleanup();
    mediaQueryCleanup = null;
  }
  mediaQueryList = null;
}

function bindSystemListener(onChange: (resolvedMode: ResolvedThemeMode) => void) {
  clearSystemListener();
  if (typeof window === "undefined") {
    return;
  }
  mediaQueryList = window.matchMedia(SYSTEM_QUERY);
  const handler = (event: MediaQueryListEvent) => {
    onChange(event.matches ? "dark" : "light");
  };
  mediaQueryList.addEventListener("change", handler);
  mediaQueryCleanup = () => {
    mediaQueryList?.removeEventListener("change", handler);
  };
}

export const useThemeStore = create<ThemeState>((set, get) => ({
  initialized: false,
  mode: "system",
  resolvedMode: "light",

  hydrate: () => {
    const mode = readStoredMode();
    const resolvedMode = resolveMode(mode);
    applyResolvedMode(resolvedMode);

    if (mode === "system") {
      bindSystemListener((nextResolvedMode) => {
        set({
          resolvedMode: nextResolvedMode,
        });
        applyResolvedMode(nextResolvedMode);
      });
    } else {
      clearSystemListener();
    }

    set({
      initialized: true,
      mode,
      resolvedMode,
    });
  },

  setMode: (mode) => {
    writeStoredMode(mode);
    const resolvedMode = resolveMode(mode);
    applyResolvedMode(resolvedMode);

    if (mode === "system") {
      bindSystemListener((nextResolvedMode) => {
        set({
          resolvedMode: nextResolvedMode,
        });
        applyResolvedMode(nextResolvedMode);
      });
    } else {
      clearSystemListener();
    }

    set({
      initialized: true,
      mode,
      resolvedMode,
    });
  },

  toggleMode: () => {
    const state = get();
    state.setMode(state.resolvedMode === "dark" ? "light" : "dark");
  },
}));

