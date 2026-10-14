import { describe, expect, it } from 'vitest';
import { dayLabel, displayUrl, formatCount, normalizeInput, relativeTime } from './format';

describe('format', () => {
  const now = new Date('2026-10-14T12:00:00Z');
  it('relativeTime', () => {
    expect(relativeTime('2026-10-14T12:00:02Z', now)).toBe('just now');
    expect(relativeTime('2026-10-14T11:57:00Z', now)).toBe('3 minutes ago');
    expect(relativeTime('2026-10-13T12:00:00Z', now)).toBe('yesterday');
  });
  it('formatCount', () => expect(formatCount(1234567)).toBe('1,234,567'));
  it('displayUrl strips scheme and truncates in the middle', () => {
    expect(displayUrl('https://example.com/')).toBe('example.com');
    const long = displayUrl('https://example.com/' + 'a'.repeat(100) + '/end', 30);
    expect(long.length).toBe(30);
    expect(long).toContain('…');
    expect(long.endsWith('/end')).toBe(true);
  });
  it('dayLabel uses UTC days', () => expect(dayLabel('2026-10-12T00:00:00Z')).toBe('MON 12'));
  it('normalizeInput adds https://', () => {
    expect(normalizeInput(' go.dev/doc ')).toBe('https://go.dev/doc');
    expect(normalizeInput('http://a.io')).toBe('http://a.io');
    expect(normalizeInput('')).toBe('');
  });
});
