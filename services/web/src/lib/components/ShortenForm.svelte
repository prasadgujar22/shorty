<script lang="ts">
  import { ApiError, createLink, type Link } from '../api';
  import { normalizeInput } from '../format';

  let { oncreated }: { oncreated: (link: Link) => void } = $props();

  let url = $state('');
  let slug = $state('');
  let showSlug = $state(false);
  let busy = $state(false);
  let error = $state('');
  let invalid = $state<'url' | 'slug' | null>(null);
  let urlInput: HTMLInputElement;

  async function submit(event: SubmitEvent) {
    event.preventDefault();
    if (busy) return;
    error = '';
    invalid = null;
    busy = true;
    try {
      const link = await createLink(fetch, normalizeInput(url), showSlug ? slug.trim() || undefined : undefined);
      url = '';
      slug = '';
      oncreated(link);
    } catch (e) {
      const err = e instanceof ApiError ? e : new ApiError(0, 'unknown', 'Something went wrong.');
      error = err.message;
      invalid = err.code === 'invalid_url' ? 'url' : err.code === 'invalid_slug' || err.code === 'slug_taken' ? 'slug' : null;
      if (invalid === 'url') urlInput?.focus();
    } finally {
      busy = false;
    }
  }
</script>

<form class="shorten" onsubmit={submit} novalidate>
  <label class="label" for="url">Paste a long URL</label>
  <div class="row">
    <input
      bind:this={urlInput}
      bind:value={url}
      id="url"
      name="url"
      type="text"
      inputmode="url"
      autocomplete="off"
      autocapitalize="off"
      spellcheck="false"
      placeholder="https://example.com/a/very/long/path"
      aria-invalid={invalid === 'url'}
      aria-describedby={error ? 'form-error' : undefined}
      required
    />
    <button class="primary submit" type="submit" disabled={busy || !url.trim()}>
      {busy ? 'Shortening…' : 'Shorten'}
    </button>
  </div>

  <div class="options">
    <button
      type="button"
      class="text-toggle"
      aria-expanded={showSlug}
      aria-controls="slug-field"
      onclick={() => (showSlug = !showSlug)}
    >
      {showSlug ? '− Custom slug' : '+ Custom slug'}
    </button>
  </div>

  {#if showSlug}
    <div class="slug" id="slug-field">
      <label class="label" for="slug">Custom slug (optional)</label>
      <div class="slug-row">
        <span aria-hidden="true">/</span>
        <input
          bind:value={slug}
          id="slug"
          name="slug"
          type="text"
          autocomplete="off"
          autocapitalize="off"
          spellcheck="false"
          maxlength="32"
          placeholder="launch-2026"
          aria-invalid={invalid === 'slug'}
          aria-describedby="slug-hint"
        />
      </div>
      <p class="hint" id="slug-hint">3–32 letters, digits, - or _</p>
    </div>
  {/if}

  <p class="error" id="form-error" role="alert" aria-live="assertive">
    {#if error}<strong>ERROR</strong> {error}{/if}
  </p>
</form>

<style>
  .shorten {
    display: grid;
    gap: var(--s3);
  }
  .row {
    display: flex;
    gap: var(--s3);
    align-items: stretch;
  }
  .row input {
    font-size: clamp(18px, 3.2vw, 28px);
    padding: var(--s3) 0;
    border-bottom-width: 2px;
  }
  .row input:focus-visible {
    border-bottom-width: 4px;
    padding-bottom: calc(var(--s3) - 2px);
  }
  .submit {
    min-width: 160px;
    justify-content: center;
    font-size: 14px;
    padding: 0 var(--s4);
  }
  .options {
    display: flex;
    gap: var(--s3);
  }
  .text-toggle {
    border: 0;
    padding: var(--s1) 0;
    min-height: 0;
    background: none;
    color: var(--fg);
    text-decoration: underline;
    text-underline-offset: 4px;
  }
  .text-toggle:hover,
  .text-toggle:focus-visible {
    background: var(--fg);
    color: var(--bg);
  }
  .slug {
    display: grid;
    gap: var(--s2);
    max-width: 420px;
    border-left: var(--rule);
    padding-left: var(--s3);
  }
  .slug-row {
    display: flex;
    align-items: baseline;
    gap: var(--s1);
    font-size: 18px;
  }
  .slug-row input {
    font-size: 18px;
  }
  .hint {
    font-size: 12px;
  }
  .error {
    min-height: 1.5em;
    font-size: 13px;
  }
  .error strong {
    background: var(--fg);
    color: var(--bg);
    padding: 1px 6px;
    margin-right: var(--s2);
    letter-spacing: 0.12em;
    font-size: 11px;
  }
  @media (max-width: 640px) {
    .row {
      flex-direction: column;
    }
    .submit {
      min-height: 52px;
    }
  }
</style>
