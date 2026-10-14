<script lang="ts">
  import { resolve } from '$app/paths';
  import { page } from '$app/state';
  import { onMount } from 'svelte';
  import { ApiError, getLink, getStats, type Link, type Stats } from '../../../lib/api';
  import BarChart from '../../../lib/components/BarChart.svelte';
  import CopyButton from '../../../lib/components/CopyButton.svelte';
  import Referrers from '../../../lib/components/Referrers.svelte';
  import Sparkline from '../../../lib/components/Sparkline.svelte';
  import { formatCount, relativeTime } from '../../../lib/format';

  const REFRESH_MS = 15_000;

  let slug = $derived(page.params.slug ?? '');
  let link = $state<Link | null>(null);
  let stats = $state<Stats | null>(null);
  let error = $state<ApiError | null>(null);
  let statsError = $state('');
  let updated = $state<Date | null>(null);
  let refreshing = $state(false);

  let last24 = $derived(stats?.hourly.reduce((s, p) => s + p.clicks, 0) ?? 0);
  let last7 = $derived(stats?.daily.reduce((s, p) => s + p.clicks, 0) ?? 0);

  async function loadLink(s: string) {
    error = null;
    link = null;
    stats = null;
    try {
      link = await getLink(fetch, s);
      await refresh();
    } catch (e) {
      error = e instanceof ApiError ? e : new ApiError(0, 'unknown', 'Something went wrong.');
    }
  }

  async function refresh() {
    if (!link || refreshing) return;
    refreshing = true;
    try {
      stats = await getStats(fetch, link.slug);
      statsError = '';
      updated = new Date();
    } catch (e) {
      // Analytics being down must not break the page: keep the last numbers.
      statsError = e instanceof ApiError ? e.message : 'Stats unavailable.';
    } finally {
      refreshing = false;
    }
  }

  $effect(() => {
    loadLink(slug);
  });

  onMount(() => {
    const timer = setInterval(() => {
      if (document.visibilityState === 'visible') refresh();
    }, REFRESH_MS);
    return () => clearInterval(timer);
  });
</script>

<svelte:head><title>SHORTY — /{slug}</title></svelte:head>

<p class="crumb"><a href={resolve('/')}>← All links</a></p>

{#if error}
  <section class="missing" aria-labelledby="missing-title">
    <h1 id="missing-title">{error.status === 404 ? 'Not found' : 'Unavailable'}</h1>
    <p>{error.status === 404 ? `There is no short link /${slug}.` : error.message}</p>
    <a class="button" href={resolve('/')}>Shorten a link</a>
  </section>
{:else if !link}
  <p class="label" aria-live="polite">Loading /{slug}…</p>
{:else}
  <header class="head">
    <p class="label">Short link</p>
    <!-- eslint-disable-next-line svelte/no-navigation-without-resolve -- external URL -->
    <h1><a href={link.short_url} target="_blank" rel="noopener">{link.short_url.replace(/^https?:\/\//, '')}</a></h1>
    <!-- eslint-disable-next-line svelte/no-navigation-without-resolve -- external URL -->
    <p class="target">→ <a href={link.url} rel="noopener noreferrer" target="_blank">{link.url}</a></p>
    <div class="actions">
      <CopyButton text={link.short_url} />
      <button type="button" onclick={refresh} disabled={refreshing}>{refreshing ? 'Refreshing…' : 'Refresh'}</button>
      <span class="label updated" aria-live="polite">
        {#if statsError}Stats unavailable — {statsError}{:else if updated}Updated {updated.toLocaleTimeString('en-GB')}{/if}
      </span>
    </div>
  </header>

  <section class="kpis" aria-label="Summary">
    <div class="kpi primary">
      <span class="label">Total clicks</span>
      <span class="num mono-num">{formatCount(stats?.total ?? 0)}</span>
    </div>
    <div class="kpi">
      <span class="label">Last 24h</span>
      <span class="num mono-num">{formatCount(last24)}</span>
    </div>
    <div class="kpi">
      <span class="label">Last 7 days</span>
      <span class="num mono-num">{formatCount(last7)}</span>
    </div>
    <div class="kpi">
      <span class="label">Last click</span>
      <span class="num small">{stats?.last_click_at ? relativeTime(stats.last_click_at) : '—'}</span>
    </div>
  </section>

  {#if stats}
    <section class="block" aria-labelledby="h24">
      <div class="block-head">
        <h2 class="label" id="h24">Last 24 hours · hourly</h2>
        <span class="label">Peak {formatCount(Math.max(0, ...stats.hourly.map((p) => p.clicks)))}</span>
      </div>
      <Sparkline points={stats.hourly} label="Clicks per hour, last 24 hours" />
    </section>

    <section class="block" aria-labelledby="d7">
      <div class="block-head">
        <h2 class="label" id="d7">Last 7 days · daily (UTC)</h2>
      </div>
      <BarChart points={stats.daily} label="Clicks per day, last 7 days" />
    </section>

    <section class="block" aria-labelledby="refs">
      <div class="block-head">
        <h2 class="label" id="refs">Top referrers</h2>
      </div>
      <Referrers referrers={stats.referrers} />
    </section>
  {/if}

  <p class="created label">Created {relativeTime(link.created_at)} · {link.custom ? 'custom slug' : 'generated slug'}</p>
{/if}

<style>
  .crumb {
    margin-bottom: var(--s4);
    font-size: 13px;
  }
  .head {
    display: grid;
    gap: var(--s3);
    padding-bottom: var(--s5);
    border-bottom: var(--rule);
  }
  h1 {
    font-size: clamp(28px, 5.5vw, 56px);
    line-height: 1.05;
    letter-spacing: -0.02em;
    word-break: break-all;
  }
  h1 a {
    text-decoration-thickness: 2px;
  }
  .target {
    font-size: 13px;
    word-break: break-all;
  }
  .actions {
    display: flex;
    gap: var(--s2);
    align-items: center;
    flex-wrap: wrap;
  }
  .updated {
    margin-left: auto;
    font-weight: 400;
  }
  .kpis {
    display: grid;
    grid-template-columns: 2fr 1fr 1fr 1fr;
    border-bottom: var(--rule);
  }
  .kpi {
    display: grid;
    gap: var(--s2);
    align-content: start;
    padding: var(--s4) var(--s3);
    border-right: var(--rule);
  }
  .kpi.primary {
    background: var(--fg);
    color: var(--bg);
  }
  .num {
    font-size: clamp(28px, 4vw, 44px);
    font-weight: 700;
    line-height: 1;
  }
  .kpi.primary .num {
    font-size: clamp(40px, 7vw, 80px);
  }
  .num.small {
    font-size: 18px;
    line-height: 1.3;
  }
  .block {
    padding: var(--s5) 0;
    border-bottom: var(--rule);
  }
  .block-head {
    display: flex;
    justify-content: space-between;
    margin-bottom: var(--s3);
  }
  .created {
    margin-top: var(--s4);
    font-weight: 400;
  }
  .missing {
    border: 2px solid var(--fg);
    padding: var(--s5);
    display: grid;
    gap: var(--s3);
    justify-items: start;
  }
  @media (max-width: 760px) {
    .kpis {
      grid-template-columns: 1fr 1fr;
    }
    .kpi.primary,
    .kpi:last-child {
      grid-column: 1 / -1;
    }
    .kpi {
      border-bottom: var(--rule);
    }
    .updated {
      margin-left: 0;
      width: 100%;
    }
  }
</style>
