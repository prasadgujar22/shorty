<script lang="ts">
  let { text, label = 'Copy' }: { text: string; label?: string } = $props();
  let copied = $state(false);
  let timer: ReturnType<typeof setTimeout> | undefined;

  async function copy() {
    try {
      await navigator.clipboard.writeText(text);
      copied = true;
      clearTimeout(timer);
      timer = setTimeout(() => (copied = false), 1500);
    } catch {
      copied = false;
    }
  }
</script>

<button type="button" onclick={copy} aria-label={`${label} ${text}`}>
  {copied ? 'Copied ✓' : label}
</button>
<span class="sr-only" aria-live="polite">{copied ? 'Copied to clipboard' : ''}</span>
