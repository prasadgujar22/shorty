<script lang="ts">
  import { resolve } from '$app/paths';
  import type { Link } from '../api';
  import { displayUrl, relativeTime } from '../format';

  let { links, highlight = '' }: { links: Link[]; highlight?: string } = $props();
</script>

<ol class="list">
  {#each links as link (link.slug)}
    <li class:fresh={link.slug === highlight}>
      <a class="row" href={resolve('/s/[slug]', { slug: link.slug })} aria-label={`Stats for /${link.slug}, pointing to ${link.url}`}>
        <span class="slug">/{link.slug}</span>
        <span class="target" title={link.url}>{displayUrl(link.url, 72)}</span>
        <span class="meta">
          {#if link.custom}<span class="tag">Custom</span>{/if}
          <time datetime={link.created_at}>{relativeTime(link.created_at)}</time>
          <span class="arrow" aria-hidden="true">→</span>
        </span>
      </a>
    </li>
  {/each}
</ol>

<style>
  .list {
    list-style: none;
    margin: 0;
    padding: 0;
    border-top: var(--rule);
  }
  li {
    border-bottom: var(--rule);
  }
  .row {
    display: grid;
    grid-template-columns: minmax(120px, 200px) 1fr auto;
    gap: var(--s3);
    align-items: baseline;
    padding: var(--s3) var(--s2);
    text-decoration: none;
  }
  .row:hover,
  .row:focus-visible {
    background: var(--fg);
    color: var(--bg);
    outline-offset: -2px;
  }
  .fresh .row {
    border-left: 6px solid var(--fg);
  }
  .slug {
    font-weight: 700;
  }
  .target {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .meta {
    display: flex;
    gap: var(--s3);
    align-items: baseline;
    font-size: 12px;
    white-space: nowrap;
  }
  .tag {
    border: 1px solid currentColor;
    padding: 0 6px;
    font-size: 10px;
    font-weight: 700;
    letter-spacing: 0.16em;
    text-transform: uppercase;
  }
  @media (max-width: 640px) {
    .row {
      grid-template-columns: 1fr auto;
    }
    .target {
      grid-column: 1 / -1;
      grid-row: 2;
      font-size: 13px;
    }
  }
</style>
