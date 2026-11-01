import { useEffect, useState } from "react";
import type { ThemePreferences } from "mygo-runtime";
import { watchPreferences } from "./prefs";

/** colorMode maps the app's theme setting to the attribute Primer's theme
 * CSS selects on. */
export function colorModeOf(theme: string | undefined): "auto" | "light" | "dark" {
  if (theme === "dark") return "dark";
  if (theme === "light") return "light";
  return "auto";
}

const defaultPreferences: ThemePreferences = {
  accent: null,
  reduceMotion: false,
  reduceTransparency: false,
  highContrast: false,
  textScale: 1,
};

export interface ThemeScheme {
  mode: "auto" | "light" | "dark";
  dayScheme: string;
  nightScheme: string;
}

/** useThemeScheme follows the theme setting and the desktop's accessibility
 * settings. The theme lives on <html> rather than on a wrapper: dialogs,
 * menus and toasts are portaled to the body, and must inherit the same
 * tokens. */
export function useThemeScheme(theme: string | undefined): ThemeScheme {
  const [prefs, setPrefs] = useState<ThemePreferences>(defaultPreferences);

  useEffect(
    () =>
      watchPreferences((next) => {
        setPrefs(next);
        const root = document.documentElement;
        root.classList.toggle("reduce-motion", next.reduceMotion);
        root.style.setProperty("--text-scale", String(next.textScale));
      }),
    [],
  );

  const mode = colorModeOf(theme);
  const dayScheme = prefs.highContrast ? "light-high-contrast" : "light";
  const nightScheme = prefs.highContrast ? "dark-high-contrast" : "dark";

  useEffect(() => {
    const root = document.documentElement;
    root.setAttribute("data-color-mode", mode);
    root.setAttribute("data-light-theme", dayScheme);
    root.setAttribute("data-dark-theme", nightScheme);
  }, [mode, dayScheme, nightScheme]);

  return { mode, dayScheme, nightScheme };
}
