<script lang="ts">
  import { theme } from '../theme.svelte';
</script>

<button
  type="button"
  class="toggle"
  aria-label={`Switch to ${theme.current === 'dark' ? 'light' : 'dark'} theme`}
  onclick={() => theme.toggle()}
>
  <svg viewBox="0 0 16 16" width="14" height="14" aria-hidden="true">
    <circle cx="8" cy="8" r="6.5" fill="none" stroke="currentColor" stroke-width="1.5" />
    <path d="M8 1.5a6.5 6.5 0 0 1 0 13z" fill="currentColor" />
  </svg>
  <span>{theme.current === 'dark' ? 'DARK' : 'LIGHT'}</span>
</button>

<style>
  .toggle {
    min-height: 32px;
    padding: 4px 10px;
  }
</style>
