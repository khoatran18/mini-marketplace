// Light / dark / system theme. The choice is kept in localStorage AND in the `mm_theme` cookie so the server can
// render <html class="dark"> straight away; `themeInitScript` fixes the "system" case before first paint (no flash).

export type ThemeMode = 'light' | 'dark' | 'system';

export const THEME_KEY = 'mm_theme';
export const THEME_COOKIE = 'mm_theme';

export function isThemeMode(value: unknown): value is ThemeMode {
  return value === 'light' || value === 'dark' || value === 'system';
}

// Runs inline in <head> before hydration. Keep it dependency free and tiny.
export const themeInitScript = `(function(){try{var m=null;try{m=localStorage.getItem('${THEME_KEY}')}catch(e){}if(m!=='light'&&m!=='dark'&&m!=='system'){var c=document.cookie.match(/(?:^|; )${THEME_COOKIE}=([^;]+)/);m=c?c[1]:'system'}var d=m==='dark'||(m!=='light'&&window.matchMedia('(prefers-color-scheme: dark)').matches);var r=document.documentElement;r.classList.toggle('dark',d);r.style.colorScheme=d?'dark':'light'}catch(e){}})();`;

export function readThemeMode(): ThemeMode {
  if (typeof window === 'undefined') return 'system';
  try {
    const stored = window.localStorage.getItem(THEME_KEY);
    if (isThemeMode(stored)) return stored;
  } catch {
    // storage blocked: fall through to the cookie
  }
  const match = document.cookie.match(new RegExp(`(?:^|; )${THEME_COOKIE}=([^;]+)`));
  return match && isThemeMode(match[1]) ? match[1] : 'system';
}

export function resolveDark(mode: ThemeMode): boolean {
  if (mode === 'dark') return true;
  if (mode === 'light') return false;
  return typeof window !== 'undefined' && window.matchMedia('(prefers-color-scheme: dark)').matches;
}

export function applyTheme(mode: ThemeMode) {
  if (typeof document === 'undefined') return;
  const dark = resolveDark(mode);
  document.documentElement.classList.toggle('dark', dark);
  document.documentElement.style.colorScheme = dark ? 'dark' : 'light';
}

export function saveThemeMode(mode: ThemeMode) {
  try {
    window.localStorage.setItem(THEME_KEY, mode);
  } catch {
    // ignore: the cookie below still carries the choice
  }
  document.cookie = `${THEME_COOKIE}=${mode}; path=/; max-age=31536000; SameSite=Lax`;
  applyTheme(mode);
}
