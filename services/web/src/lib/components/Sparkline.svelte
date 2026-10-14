<script lang="ts">
  import type { Point } from '../api';
  import { sparkline } from '../chart';
  import { formatCount, hourLabel } from '../format';

  let { points, label }: { points: Point[]; label: string } = $props();

  const W = 720;
  const H = 180;
  const box = { width: W, height: H, padTop: 24, padBottom: 28, padX: 8 };

  let geo = $derived(sparkline(points.map((p) => p.clicks), box));
  let active = $state<number | null>(null);
  let svg: SVGSVGElement;

  function pick(event: PointerEvent) {
    const r = svg.getBoundingClientRect();
    const x = ((event.clientX - r.left) / r.width) * W;
    const step = (W - 2 * box.padX) / Math.max(1, points.length - 1);
    active = Math.max(0, Math.min(points.length - 1, Math.round((x - box.padX) / step)));
  }

  function keys(event: KeyboardEvent) {
    const last = points.length - 1;
    const moves: Record<string, number> = { ArrowLeft: -1, ArrowRight: 1 };
    if (event.key in moves) {
      event.preventDefault();
      active = Math.max(0, Math.min(last, (active ?? last) + moves[event.key]));
    } else if (event.key === 'Home' || event.key === 'End') {
      event.preventDefault();
      active = event.key === 'Home' ? 0 : last;
    } else if (event.key === 'Escape') {
      active = null;
    }
  }

  let total = $derived(points.reduce((s, p) => s + p.clicks, 0));
  let current = $derived(active === null ? null : { p: points[active], c: geo.coords[active] });
</script>

<figure class="chart">
  <!-- A scrubber over the 24 hours: arrow keys move, like a slider. -->
  <svg
    bind:this={svg}
    viewBox={`0 0 ${W} ${H}`}
    role="slider"
    aria-label={`${label}: ${formatCount(total)} clicks in total. Use arrow keys to inspect each hour.`}
    aria-valuemin={0}
    aria-valuemax={Math.max(0, points.length - 1)}
    aria-valuenow={active ?? points.length - 1}
    aria-valuetext={current ? `${hourLabel(current.p.t)}: ${current.p.clicks} clicks` : `${formatCount(total)} clicks in 24 hours`}
    tabindex="0"
    onpointermove={pick}
    onpointerleave={() => (active = null)}
    onkeydown={keys}
    onblur={() => (active = null)}
  >
    <!-- y scale -->
    <line x1={box.padX} x2={W - box.padX} y1={box.padTop} y2={box.padTop} class="grid" />
    <text x={W - box.padX} y={box.padTop - 8} text-anchor="end" class="axis">{formatCount(geo.max)}</text>

    {#if geo.area}
      <path d={geo.area} fill="url(#hatch-sparse)" />
      <polyline points={geo.line} fill="none" stroke="currentColor" stroke-width="2" stroke-linejoin="miter" />
    {/if}
    <line x1={box.padX} x2={W - box.padX} y1={H - box.padBottom} y2={H - box.padBottom} stroke="currentColor" stroke-width="1" />

    {#each points as p, i (p.t)}
      {#if i % 6 === 0 || i === points.length - 1}
        <text x={geo.coords[i][0]} y={H - 8} text-anchor={i === 0 ? 'start' : i === points.length - 1 ? 'end' : 'middle'} class="axis">
          {i === points.length - 1 ? 'NOW' : hourLabel(p.t)}
        </text>
      {/if}
    {/each}

    {#if current}
      <line x1={current.c[0]} x2={current.c[0]} y1={box.padTop} y2={H - box.padBottom} stroke="currentColor" stroke-width="1" />
      <rect x={current.c[0] - 4} y={current.c[1] - 4} width="8" height="8" class="dot" />
      {@const right = current.c[0] > W - 200}
      <g transform={`translate(${right ? current.c[0] - 168 : current.c[0] + 8}, ${box.padTop})`}>
        <rect width="160" height="24" class="tip" />
        <text x="8" y="16" class="tip-text">{hourLabel(current.p.t)} · {formatCount(current.p.clicks)}</text>
      </g>
    {/if}
  </svg>
  <figcaption class="sr-only">
    <table>
      <caption>{label}</caption>
      <thead><tr><th>Hour</th><th>Clicks</th></tr></thead>
      <tbody>
        {#each points as p (p.t)}<tr><td>{hourLabel(p.t)}</td><td>{p.clicks}</td></tr>{/each}
      </tbody>
    </table>
  </figcaption>
</figure>

<style>
  .chart {
    margin: 0;
  }
  svg {
    display: block;
    width: 100%;
    height: auto;
    color: var(--fg);
    touch-action: pan-y;
  }
  .grid {
    stroke: currentColor;
    stroke-width: 1;
    stroke-dasharray: 2 4;
  }
  .axis {
    fill: currentColor;
    font-size: 11px;
    font-weight: 700;
    letter-spacing: 0.12em;
  }
  .dot,
  .tip {
    fill: var(--fg);
  }
  .tip-text {
    fill: var(--bg);
    font-size: 12px;
    font-weight: 700;
  }
</style>
