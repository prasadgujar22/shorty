<script lang="ts">
  import type { Stats } from '../api';
  import { formatCount } from '../format';

  let { referrers }: { referrers: Stats['referrers'] } = $props();
  let max = $derived(Math.max(1, ...referrers.map((r) => r.clicks)));
</script>

{#if referrers.length === 0}
  <p class="empty">No referrers yet.</p>
{:else}
  <ol class="refs">
    {#each referrers as r, i (r.referrer)}
      <li>
        <span class="rank mono-num">{String(i + 1).padStart(2, '0')}</span>
        <span class="name">{r.referrer}</span>
        <span class="bar" aria-hidden="true">
          <!-- Unscaled SVG with a % width so the hatch pattern is never distorted. -->
          <svg width="100%" height="14">
            <rect x="0.5" y="0.5" width={`${Math.max(1, (r.clicks / max) * 99.5)}%`} height="13" fill={i === 0 ? 'url(#hatch)' : 'url(#hatch-sparse)'} stroke="currentColor" stroke-width="1" />
          </svg>
        </span>
        <span class="count mono-num">{formatCount(r.clicks)}</span>
      </li>
    {/each}
  </ol>
{/if}

<style>
  .refs {
    list-style: none;
    margin: 0;
    padding: 0;
    border-top: var(--rule);
  }
  li {
    display: grid;
    grid-template-columns: 32px minmax(120px, 240px) 1fr 80px;
    gap: var(--s3);
    align-items: center;
    padding: var(--s2) 0;
    border-bottom: var(--rule-dashed);
  }
  li:first-child .name {
    font-weight: 700;
  }
  .rank {
    font-size: 11px;
    font-weight: 700;
  }
  .name {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .bar svg {
    display: block;
    width: 100%;
    height: 14px;
    color: var(--fg);
  }
  .count {
    text-align: right;
    font-weight: 700;
  }
  .empty {
    border: var(--rule-dashed);
    padding: var(--s4);
    text-align: center;
  }
  @media (max-width: 640px) {
    li {
      grid-template-columns: 28px 1fr 64px;
    }
    .bar {
      grid-column: 2 / -1;
      grid-row: 2;
    }
  }
</style>
