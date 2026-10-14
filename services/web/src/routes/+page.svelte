<script lang="ts">
  import { resolve } from '$app/paths';
  import { onMount } from 'svelte';
  import { ApiError, listLinks, type Link } from '../lib/api';
  import CopyButton from '../lib/components/CopyButton.svelte';
  import LinkList from '../lib/components/LinkList.svelte';
  import ShortenForm from '../lib/components/ShortenForm.svelte';
  import { displayUrl } from '../lib/format';

  let links = $state<Link[]>([]);
  let next = $state<string | undefined>();
  let loading = $state(true);
  let loadError = $state('');
  let created = $state<Link | null>(null);
  let resultHeading = $state<HTMLElement>();

  async function load(before?: string) {
    loading = true;
    loadError = '';
    try {
      const page = await listLinks(fetch, 20, before);
      links = before ? [...links, ...page.links] : page.links;
      next = page.next_before;
    } catch (e) {
      loadError = e instanceof ApiError ? e.message : 'Could not load links.';
    } finally {
      loading = false;
    }
  }

  async function onCreated(link: Link) {
    created = link;
    links = [link, ...links.filter((l) => l.slug !== link.slug)];
    // Move focus to the result so keyboard and screen-reader users land on it.
    await Promise.resolve();
    resultHeading?.focus();
  }

  onMount(() => {
    load();
  });
</script>

<svelte:head><title>SHORTY — shorten a link</title></svelte:head>

<section class="hero" aria-labelledby="hero-title">
  <h1 id="hero-title">Long links in.<br />Short links out.</h1>
  <ShortenForm oncreated={onCreated} />
</section>

{#if created}
  <section class="result" aria-labelledby="result-title">
    <h2 class="label" id="result-title" tabindex="-1" bind:this={resultHeading}>Your short link</h2>
    <!-- eslint-disable-next-line svelte/no-navigation-without-resolve -- external URL -->
    <p class="short"><a href={created.short_url} target="_blank" rel="noopener">{created.short_url.replace(/^https?:\/\//, '')}</a></p>
    <p class="target">→ {displayUrl(created.url, 90)}</p>
    <div class="actions">
      <CopyButton text={created.short_url} />
      <a class="button" href={resolve('/s/[slug]', { slug: created.slug })}>Stats →</a>
    </div>
  </section>
{/if}

<section class="recent" aria-labelledby="recent-title" aria-busy={loading}>
  <div class="section-head">
    <h2 class="label" id="recent-title">Recent links</h2>
    <span class="label mono-num">{links.length}{next ? '+' : ''}</span>
  </div>

  {#if loadError}
    <p class="notice" role="alert">{loadError} <button type="button" onclick={() => load()}>Retry</button></p>
  {:else if !loading && links.length === 0}
    <p class="notice">Nothing here yet. Shorten your first link above.</p>
  {:else}
    <LinkList {links} highlight={created?.slug} />
  {/if}

  {#if next}
    <div class="more">
      <button type="button" onclick={() => load(next)} disabled={loading}>{loading ? 'Loading…' : 'Load more'}</button>
    </div>
  {/if}
</section>

<style>
  .hero {
    display: grid;
    gap: var(--s5);
    margin-bottom: var(--s6);
  }
  h1 {
    font-size: clamp(32px, 6vw, 64px);
    line-height: 1.05;
    font-weight: 700;
    letter-spacing: -0.03em;
  }
  .result {
    border: 2px solid var(--fg);
    padding: var(--s4);
    margin-bottom: var(--s6);
    display: grid;
    gap: var(--s3);
  }
  .result h2:focus {
    outline: none;
  }
  .short {
    font-size: clamp(22px, 4vw, 36px);
    font-weight: 700;
    word-break: break-all;
  }
  .short a {
    text-decoration-thickness: 2px;
  }
  .target {
    font-size: 13px;
    word-break: break-all;
  }
  .actions {
    display: flex;
    gap: var(--s2);
    flex-wrap: wrap;
  }
  .section-head {
    display: flex;
    justify-content: space-between;
    margin-bottom: var(--s3);
  }
  .notice {
    border: var(--rule-dashed);
    padding: var(--s4);
    display: flex;
    gap: var(--s3);
    align-items: center;
    justify-content: center;
  }
  .more {
    display: flex;
    justify-content: center;
    margin-top: var(--s4);
  }
</style>
