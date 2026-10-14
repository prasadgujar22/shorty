<script lang="ts">
  import type { Point } from '../api';
  import { bars } from '../chart';
  import { dayLabel, formatCount } from '../format';

  let { points, label }: { points: Point[]; label: string } = $props();

  const W = 720;
  const H = 220;
  const box = { width: W, height: H, padTop: 28, padBottom: 32, padX: 0 };
  let rects = $derived(bars(points.map((p) => p.clicks), box, 12));
  let active = $state<number | null>(null);
</script>

<figure class="chart">
  <svg viewBox={`0 0 ${W} ${H}`} role="group" aria-label={label}>
    <line x1="0" x2={W} y1={H - box.padBottom} y2={H - box.padBottom} stroke="currentColor" stroke-width="1" />
    {#each rects as r, i (points[i].t)}
      {@const on = active === i}
      {@const today = i === rects.length - 1}
      <!-- svelte-ignore a11y_no_noninteractive_tabindex -->
      <g
        class="bar"
        class:on
        tabindex="0"
        role="img"
        aria-label={`${dayLabel(points[i].t)}${today ? ' (today)' : ''}: ${formatCount(r.v)} clicks`}
        onpointerenter={() => (active = i)}
        onpointerleave={() => (active = null)}
        onfocus={() => (active = i)}
        onblur={() => (active = null)}
      >
        <!-- full-height hit area so tiny bars are still easy to target -->
        <rect x={r.x} y={box.padTop} width={r.w} height={H - box.padTop - box.padBottom} class="hit" />
        {#if r.h > 0}
          <rect x={r.x + 0.5} y={r.y + 0.5} width={Math.max(0, r.w - 1)} height={Math.max(0, r.h - 1)} class="fill" fill={on ? 'currentColor' : today ? 'url(#hatch)' : 'url(#hatch-sparse)'} />
        {/if}
        <text x={r.x + r.w / 2} y={(r.h > 0 ? r.y : H - box.padBottom) - 8} text-anchor="middle" class="value">{formatCount(r.v)}</text>
        <text x={r.x + r.w / 2} y={H - 10} text-anchor="middle" class="day" class:today>{today ? 'TODAY' : dayLabel(points[i].t)}</text>
      </g>
    {/each}
  </svg>
  <figcaption class="sr-only">
    <table>
      <caption>{label}</caption>
      <thead><tr><th>Day (UTC)</th><th>Clicks</th></tr></thead>
      <tbody>
        {#each points as p (p.t)}<tr><td>{dayLabel(p.t)}</td><td>{p.clicks}</td></tr>{/each}
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
  }
  .hit {
    fill: none;
    pointer-events: all;
  }
  .fill {
    stroke: currentColor;
    stroke-width: 1;
  }
  .bar:focus {
    outline: none;
  }
  .bar:focus-visible .hit {
    stroke: currentColor;
    stroke-width: 2;
    stroke-dasharray: 4 3;
  }
  .value,
  .day {
    fill: currentColor;
    font-size: 12px;
    font-weight: 700;
    letter-spacing: 0.1em;
  }
  .value {
    font-weight: 400;
  }
  .on .value {
    font-weight: 700;
  }
  .day.today {
    text-decoration: underline;
  }
</style>
