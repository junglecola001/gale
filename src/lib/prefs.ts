import { isMyGo, onPreferences, preferences, type ThemePreferences } from "mygo-runtime";

/** getPreferences is the desktop's accessibility settings, or the defaults
 * outside a MyGo window. */
export function getPreferences(): ThemePreferences {
  if (!isMyGo()) {
    return { accent: null, reduceMotion: false, reduceTransparency: false, highContrast: false, textScale: 1 };
  }
  return preferences();
}

/** watchPreferences calls listener with the desktop's settings now and on
 * every change; it returns an unsubscribe function. */
export function watchPreferences(listener: (p: ThemePreferences) => void): () => void {
  listener(getPreferences());
  if (!isMyGo()) return () => {};
  return onPreferences(listener);
}
