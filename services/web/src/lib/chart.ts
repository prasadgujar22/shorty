// Geometry for the pure-SVG charts. No chart library, no DOM: unit tested.

export interface Box {
  width: number;
  height: number;
  padTop: number;
  padBottom: number;
  padX: number;
}

/** A "nice" ceiling for the y axis so labels read well (1, 2, 5 × 10^n). */
export function niceMax(max: number): number {
  if (max <= 0) return 1;
  const exp = Math.pow(10, Math.floor(Math.log10(max)));
  for (const m of [1, 2, 5, 10]) {
    if (max <= m * exp) return m * exp;
  }
  return 10 * exp;
}

/** Points for a sparkline; returns the polyline and the closed area path. */
export function sparkline(values: number[], box: Box): { line: string; area: string; coords: [number, number][]; max: number } {
  const max = niceMax(Math.max(0, ...values));
  const innerW = box.width - 2 * box.padX;
  const innerH = box.height - box.padTop - box.padBottom;
  const base = box.height - box.padBottom;
  const step = values.length > 1 ? innerW / (values.length - 1) : 0;
  const coords = values.map((v, i): [number, number] => [
    round(box.padX + i * step),
    round(base - (v / max) * innerH)
  ]);
  const line = coords.map(([x, y]) => `${x},${y}`).join(' ');
  const area = coords.length
    ? `M${coords[0][0]},${base} ` + coords.map(([x, y]) => `L${x},${y}`).join(' ') + ` L${coords[coords.length - 1][0]},${base} Z`
    : '';
  return { line, area, coords, max };
}

/** Bars laid out left to right with a fixed gap. */
export function bars(values: number[], box: Box, gap = 8): { x: number; y: number; w: number; h: number; v: number }[] {
  const max = niceMax(Math.max(0, ...values));
  const innerW = box.width - 2 * box.padX;
  const innerH = box.height - box.padTop - box.padBottom;
  const base = box.height - box.padBottom;
  const w = values.length ? (innerW - gap * (values.length - 1)) / values.length : 0;
  return values.map((v, i) => {
    // Non-zero values get at least 2px so they are visible against the baseline.
    const h = v > 0 ? Math.max(2, (v / max) * innerH) : 0;
    return { x: round(box.padX + i * (w + gap)), y: round(base - h), w: round(w), h: round(h), v };
  });
}

const round = (n: number) => Math.round(n * 100) / 100;
