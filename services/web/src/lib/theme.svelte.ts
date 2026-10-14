// Theme state: follows prefers-color-scheme until the user picks one, then
// persists the choice. Only ever 'light' (white bg) or 'dark' (black bg).
export type Theme = 'light' | 'dark';

const KEY = 'shorty-theme';

function system(): Theme {
  return typeof matchMedia === 'function' && matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light';
}

function saved(): Theme | null {
  try {
    const t = localStorage.getItem(KEY);
    return t === 'light' || t === 'dark' ? t : null;
  } catch {
    return null;
  }
}

class ThemeState {
  current = $state<Theme>(saved() ?? system());

  constructor() {
    if (typeof matchMedia === 'function') {
      matchMedia('(prefers-color-scheme: dark)').addEventListener('change', () => {
        if (!saved()) this.current = system();
      });
    }
  }

  toggle() {
    this.current = this.current === 'dark' ? 'light' : 'dark';
    document.documentElement.dataset.theme = this.current;
    try {
      localStorage.setItem(KEY, this.current);
    } catch {
      /* private mode: still switches for this page view */
    }
  }
}

export const theme = new ThemeState();
