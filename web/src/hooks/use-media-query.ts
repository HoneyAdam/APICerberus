import { useEffect, useState } from "react";
import { BREAKPOINTS } from "@/lib/constants";

export function useMediaQuery(query: string, defaultValue = false) {
  const [matches, setMatches] = useState(defaultValue);

  useEffect(() => {
    if (typeof window === "undefined") {
      return;
    }
    const mediaQuery = window.matchMedia(query);
    const onChange = (event: MediaQueryListEvent) => {
      setMatches(event.matches);
    };
    setMatches(mediaQuery.matches);
    mediaQuery.addEventListener("change", onChange);
    return () => {
      mediaQuery.removeEventListener("change", onChange);
    };
  }, [query]);

  return matches;
}

export function useMinWidth(minWidth: number) {
  return useMediaQuery(`(min-width: ${minWidth}px)`);
}

export function useBreakpoint(name: keyof typeof BREAKPOINTS) {
  return useMinWidth(BREAKPOINTS[name]);
}

