/**
 * Dynamically manages the highlight.js stylesheet to match the app theme.
 * Instead of statically importing one theme, we toggle between light and dark.
 */

import { useAppStore } from '../store/appStore';

const LIGHT_THEME = 'highlight.js/styles/github.css';
const DARK_THEME = 'highlight.js/styles/github-dark.css';

let currentThemeLink: HTMLLinkElement | null = null;
let initialThemeLoaded = false;

function isDarkMode(): boolean {
  const root = document.documentElement;
  if (root.classList.contains('dark')) return true;
  if (root.classList.contains('light')) return false;
  return window.matchMedia('(prefers-color-scheme: dark)').matches;
}

function injectThemeCSS(dark: boolean): void {
  const href = dark ? DARK_THEME : LIGHT_THEME;

  if (!currentThemeLink) {
    // First call: create the link element
    currentThemeLink = document.createElement('link');
    currentThemeLink.rel = 'stylesheet';
    currentThemeLink.id = 'hljs-theme';
    document.head.appendChild(currentThemeLink);
  }

  // Only update href if theme actually changed
  const fullHref = new URL(href, import.meta.url).pathname;
  if (currentThemeLink.getAttribute('data-theme') === (dark ? 'dark' : 'light')) {
    return;
  }

  currentThemeLink.href = fullHref;
  currentThemeLink.setAttribute('data-theme', dark ? 'dark' : 'light');
}

/**
 * Ensure the correct highlight.js theme is loaded.
 * Call this once on app init and whenever the theme changes.
 */
export function syncHljsTheme(): void {
  injectThemeCSS(isDarkMode());
}

/**
 * Initialize highlight.js theme management.
 * Subscribes to the app store for theme changes.
 * Returns an unsubscribe function.
 */
export function initHljsTheme(): () => void {
  // Load initial theme
  if (!initialThemeLoaded) {
    syncHljsTheme();
    initialThemeLoaded = true;
  }

  // Subscribe to theme changes in the store
  const unsubscribe = useAppStore.subscribe((state, prevState) => {
    if (state.theme !== prevState.theme) {
      // Defer to next frame so setTheme has applied CSS classes
      requestAnimationFrame(() => syncHljsTheme());
    }
  });

  // Also listen for system theme changes
  const mq = window.matchMedia('(prefers-color-scheme: dark)');
  const handleSystemThemeChange = () => {
    const { theme } = useAppStore.getState();
    if (theme === 'system') {
      requestAnimationFrame(() => syncHljsTheme());
    }
  };
  mq.addEventListener('change', handleSystemThemeChange);

  return () => {
    unsubscribe();
    mq.removeEventListener('change', handleSystemThemeChange);
  };
}
