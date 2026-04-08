import { createContext, useContext, useEffect, useMemo, type PropsWithChildren } from "react";
import { useThemeStore, type ResolvedThemeMode, type ThemeMode, type ThemeColorScheme } from "@/stores/theme";

type ThemeContextValue = {
  mode: ThemeMode;
  resolvedMode: ResolvedThemeMode;
  colorScheme: ThemeColorScheme;
  animationEnabled: boolean;
  systemPreferenceDetected: boolean;
  setMode: (mode: ThemeMode) => void;
  toggleMode: () => void;
  setColorScheme: (scheme: ThemeColorScheme) => void;
  setAnimationEnabled: (enabled: boolean) => void;
};

const ThemeContext = createContext<ThemeContextValue | null>(null);

export function ThemeProvider({ children }: PropsWithChildren) {
  const hydrate = useThemeStore((state) => state.hydrate);
  const mode = useThemeStore((state) => state.mode);
  const resolvedMode = useThemeStore((state) => state.resolvedMode);
  const colorScheme = useThemeStore((state) => state.colorScheme);
  const animationEnabled = useThemeStore((state) => state.animationEnabled);
  const systemPreferenceDetected = useThemeStore((state) => state.systemPreferenceDetected);
  const setMode = useThemeStore((state) => state.setMode);
  const toggleMode = useThemeStore((state) => state.toggleMode);
  const setColorScheme = useThemeStore((state) => state.setColorScheme);
  const setAnimationEnabled = useThemeStore((state) => state.setAnimationEnabled);

  useEffect(() => {
    hydrate();
  }, [hydrate]);

  const value = useMemo<ThemeContextValue>(
    () => ({
      mode,
      resolvedMode,
      colorScheme,
      animationEnabled,
      systemPreferenceDetected,
      setMode,
      toggleMode,
      setColorScheme,
      setAnimationEnabled,
    }),
    [mode, resolvedMode, colorScheme, animationEnabled, systemPreferenceDetected, setMode, toggleMode, setColorScheme, setAnimationEnabled],
  );

  return <ThemeContext.Provider value={value}>{children}</ThemeContext.Provider>;
}

export function useTheme() {
  const context = useContext(ThemeContext);
  if (!context) {
    throw new Error("useTheme must be used within ThemeProvider");
  }
  return context;
}

