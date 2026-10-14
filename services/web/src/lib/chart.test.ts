import { describe, expect, it } from 'vitest';
import { bars, niceMax, sparkline } from './chart';

const box = { width: 100, height: 60, padTop: 10, padBottom: 10, padX: 0 };

describe('niceMax', () => {
  it.each([
    [0, 1],
    [1, 1],
    [3, 5],
    [7, 10],
    [11, 20],
    [480, 500],
    [501, 1000]
  ])('%d -> %d', (max, want) => expect(niceMax(max)).toBe(want));
});

describe('sparkline', () => {
  it('maps values into the inner box, baseline at the bottom', () => {
    const s = sparkline([0, 5, 10], box);
    expect(s.max).toBe(10);
    expect(s.coords).toEqual([
      [0, 50],
      [50, 30],
      [100, 10]
    ]);
    expect(s.area.startsWith('M0,50')).toBe(true);
    expect(s.area.endsWith('L100,50 Z')).toBe(true);
  });
  it('handles all zeros and empty input', () => {
    expect(sparkline([0, 0], box).coords.every(([, y]) => y === 50)).toBe(true);
    expect(sparkline([], box).area).toBe('');
  });
});

describe('bars', () => {
  it('lays out bars with gaps and a minimum visible height', () => {
    const b = bars([0, 1, 100], { ...box, width: 116 }, 8);
    expect(b.map((r) => r.w)).toEqual([33.33, 33.33, 33.33]);
    expect(b[0].h).toBe(0);
    expect(b[1].h).toBe(2);
    expect(b[2]).toMatchObject({ y: 10, h: 40 });
    expect(b[2].x).toBeCloseTo(82.67, 1);
  });
});
