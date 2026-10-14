// Formatting helpers. Pure functions, unit tested.

const units: [Intl.RelativeTimeFormatUnit, number][] = [
  ['year', 365 * 24 * 3600],
  ['month', 30 * 24 * 3600],
  ['day', 24 * 3600],
  ['hour', 3600],
  ['minute', 60],
  ['second', 1]
];

/** "3 minutes ago" style relative time. */
export function relativeTime(iso: string, now: Date = new Date()): string {
  const diff = (new Date(iso).getTime() - now.getTime()) / 1000;
  const abs = Math.abs(diff);
  if (abs < 5) return 'just now';
  const rtf = new Intl.RelativeTimeFormat('en', { numeric: 'auto' });
  for (const [unit, secs] of units) {
    if (abs >= secs || unit === 'second') return rtf.format(Math.round(diff / secs), unit);
  }
  return '';
}

/** 1234567 -> "1,234,567" */
export const formatCount = (n: number): string => new Intl.NumberFormat('en').format(n);

/** Strip the scheme and trailing slash and truncate in the middle. */
export function displayUrl(url: string, max = 64): string {
  let s = url.replace(/^https?:\/\//, '').replace(/\/$/, '');
  if (s.length > max) {
    const head = Math.ceil((max - 1) * 0.6);
    s = s.slice(0, head) + '…' + s.slice(s.length - (max - 1 - head));
  }
  return s;
}

/** "14:00" for hourly buckets (UTC from the API, shown in local time). */
export const hourLabel = (iso: string): string =>
  new Date(iso).toLocaleTimeString('en-GB', { hour: '2-digit', minute: '2-digit' });

/** "MON 13" for daily buckets (the API buckets by UTC day). */
export const dayLabel = (iso: string): string => {
  const d = new Date(iso);
  return d.toLocaleDateString('en-GB', { weekday: 'short', timeZone: 'UTC' }).toUpperCase() + ' ' + d.getUTCDate();
};

/** Ensure the user typed something URL-like; prefix https:// if no scheme. */
export function normalizeInput(raw: string): string {
  const s = raw.trim();
  if (!s) return s;
  return /^[a-z][a-z0-9+.-]*:\/\//i.test(s) ? s : 'https://' + s;
}
