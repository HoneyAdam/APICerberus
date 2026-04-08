import { create } from "zustand";
import { STORAGE_KEYS } from "@/lib/constants";

export type ThemeMode = "system" | "light" | "dark";
export type ResolvedThemeMode = "light" | "dark";
export type ThemeColorScheme = "default" | "blue" | "purple" | "green" | "amber";

type ThemeState = {
  initialized: boolean;
  mode: ThemeMode;
  resolvedMode: ResolvedThemeMode;
  colorScheme: ThemeColorScheme;
  animationEnabled: boolean;
  systemPreferenceDetected: boolean;
  hydrate: () => void;
  setMode: (mode: ThemeMode) => void;
  toggleMode: () => void;
  setColorScheme: (scheme: ThemeColorScheme) => void;
  setAnimationEnabled: (enabled: boolean) => void;
};

const SYSTEM_QUERY = "(prefers-color-scheme: dark)";
const MOTION_QUERY = "(prefers-reduced-motion: reduce)";
let mediaQueryList: MediaQueryList | null = null;
let motionQueryList: MediaQueryList | null = null;
let mediaQueryCleanup: (() => void) | null = null;
let motionQueryCleanup: (() => void) | null = null;

function isThemeMode(value: string | null): value is ThemeMode {
  return value === "system" || value === "light" || value === "dark";
}

function isThemeColorScheme(value: string | null): value is ThemeColorScheme {
  return value === "default" || value === "blue" || value === "purple" || value === "green" || value === "amber";
}

function readStoredMode(): ThemeMode {
  if (typeof window === "undefined") {
    return "system";
  }
  const value = window.localStorage.getItem(STORAGE_KEYS.themeMode);
  return isThemeMode(value) ? value : "system";
}

function readStoredColorScheme(): ThemeColorScheme {
  if (typeof window === "undefined") {
    return "default";
  }
  const value = window.localStorage.getItem("apicerberus.theme_color_scheme");
  return isThemeColorScheme(value) ? value : "default";
}

function readStoredAnimationEnabled(): boolean {
  if (typeof window === "undefined") {
    return true;
  }
  const value = window.localStorage.getItem("apicerberus.theme_animation");
  return value !== "false";
}

function writeStoredMode(mode: ThemeMode) {
  if (typeof window === "undefined") {
    return;
  }
  window.localStorage.setItem(STORAGE_KEYS.themeMode, mode);
}

function writeStoredColorScheme(scheme: ThemeColorScheme) {
  if (typeof window === "undefined") {
    return;
  }
  window.localStorage.setItem("apicerberus.theme_color_scheme", scheme);
}

function writeStoredAnimationEnabled(enabled: boolean) {
  if (typeof window === "undefined") {
    return;
  }
  window.localStorage.setItem("apicerberus.theme_animation", enabled ? "true" : "false");
}

function detectSystemMode(): ResolvedThemeMode {
  if (typeof window === "undefined") {
    return "light";
  }
  return window.matchMedia(SYSTEM_QUERY).matches ? "dark" : "light";
}

function detectReducedMotion(): boolean {
  if (typeof window === "undefined") {
    return false;
  }
  return window.matchMedia(MOTION_QUERY).matches;
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

  // Add transition class for smooth theme change
  root.classList.add("theme-transition");

  // Apply theme
  root.classList.toggle("dark", mode === "dark");
  root.setAttribute("data-theme", mode);

  // Update meta theme-color for mobile browsers
  const metaThemeColor = document.querySelector('meta[name="theme-color"]');
  if (metaThemeColor) {
    metaThemeColor.setAttribute(
      "content",
      mode === "dark" ? "hsl(222.2 84% 4.9%)" : "hsl(0 0% 100%)"
    );
  }

  // Remove transition class after animation completes
  setTimeout(() => {
    root.classList.remove("theme-transition");
  }, 300);
}

function applyColorScheme(scheme: ThemeColorScheme) {
  if (typeof document === "undefined") {
    return;
  }
  const root = document.documentElement;
  root.setAttribute("data-color-scheme", scheme);
}

function applyAnimationPreference(enabled: boolean) {
  if (typeof document === "undefined") {
    return;
  }
  const root = document.documentElement;
  if (enabled) {
    root.classList.remove("reduce-motion");
  } else {
    root.classList.add("reduce-motion");
  }
}

function clearSystemListener() {
  if (mediaQueryCleanup) {
    mediaQueryCleanup();
    mediaQueryCleanup = null;
  }
  mediaQueryList = null;
}

function clearMotionListener() {
  if (motionQueryCleanup) {
    motionQueryCleanup();
    motionQueryCleanup = null;
  }
  motionQueryList = null;
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

function bindMotionListener(onChange: (reducedMotion: boolean) => void) {
  clearMotionListener();
  if (typeof window === "undefined") {
    return;
  }
  motionQueryList = window.matchMedia(MOTION_QUERY);
  const handler = (event: MediaQueryListEvent) => {
    onChange(event.matches);
  };
  motionQueryList.addEventListener("change", handler);
  motionQueryCleanup = () => {
    motionQueryList?.removeEventListener("change", handler);
  };
}

export const useThemeStore = create<ThemeState>((set, get) => ({
  initialized: false,
  mode: "system",
  resolvedMode: "light",
  colorScheme: "default",
  animationEnabled: true,
  systemPreferenceDetected: false,

  hydrate: () => {
    const mode = readStoredMode();
    const resolvedMode = resolveMode(mode);
    const colorScheme = readStoredColorScheme();
    const animationEnabled = readStoredAnimationEnabled();
    const systemPreferenceDetected = mode === "system";

    applyResolvedMode(resolvedMode);
    applyColorScheme(colorScheme);
    applyAnimationPreference(animationEnabled && !detectReducedMotion());

    if (mode === "system") {
      bindSystemListener((nextResolvedMode) => {
        set({
          resolvedMode: nextResolvedMode,
        });
        applyResolvedMode(nextResolvedMode);
      });
    }

    // Listen for reduced motion preference
    bindMotionListener((reducedMotion) => {
      applyAnimationPreference(get().animationEnabled && !reducedMotion);
    });

    set({
      initialized: true,
      mode,
      resolvedMode,
      colorScheme,
      animationEnabled,
      systemPreferenceDetected,
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
      systemPreferenceDetected: mode === "system",
    });
  },

  toggleMode: () => {
    const state = get();
    state.setMode(state.resolvedMode === "dark" ? "light" : "dark");
  },

  setColorScheme: (scheme) => {
    writeStoredColorScheme(scheme);
    applyColorScheme(scheme);
    set({ colorScheme: scheme });
  },

  setAnimationEnabled: (enabled) => {
    writeStoredAnimationEnabled(enabled);
    applyAnimationPreference(enabled && !detectReducedMotion());
    set({ animationEnabled: enabled });
  },
}));

