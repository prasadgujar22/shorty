<script lang="ts">
  import { resolve } from '$app/paths';
  import '../app.css';
  import HatchDefs from '../lib/components/HatchDefs.svelte';
  import ThemeToggle from '../lib/components/ThemeToggle.svelte';

  let { children } = $props();
</script>

<a class="skip-link" href="#main">Skip to content</a>
<HatchDefs />

<div class="frame">
  <header class="top">
    <a class="wordmark" href={resolve('/')} aria-label="Shorty home">SHORTY<span aria-hidden="true">_</span></a>
    <nav aria-label="Primary">
      <ThemeToggle />
    </nav>
  </header>

  <main id="main" tabindex="-1">
    {@render children()}
  </main>

  <footer class="bottom">
    <span class="label">Shorty · URL shortener with click analytics</span>
    <span class="label">302 · Redis · JetStream · Postgres</span>
  </footer>
</div>

<style>
  .frame {
    max-width: var(--measure);
    margin: 0 auto;
    padding: 0 var(--s4);
    min-height: 100vh;
    display: flex;
    flex-direction: column;
  }
  .top {
    display: flex;
    justify-content: space-between;
    align-items: center;
    padding: var(--s4) 0;
    border-bottom: var(--rule);
  }
  .wordmark {
    font-weight: 700;
    font-size: 18px;
    letter-spacing: 0.28em;
    text-decoration: none;
    padding: var(--s1) var(--s2);
    margin-left: calc(-1 * var(--s2));
  }
  .wordmark:hover,
  .wordmark:focus-visible {
    background: var(--fg);
    color: var(--bg);
  }
  @media (prefers-reduced-motion: no-preference) {
    .wordmark span {
      animation: blink 1.1s steps(1) infinite;
    }
  }
  @keyframes blink {
    50% {
      visibility: hidden;
    }
  }
  main {
    flex: 1;
    padding: var(--s6) 0 var(--s7);
  }
  main:focus {
    outline: none;
  }
  .bottom {
    display: flex;
    justify-content: space-between;
    gap: var(--s3);
    flex-wrap: wrap;
    padding: var(--s4) 0;
    border-top: var(--rule);
  }
  @media (max-width: 640px) {
    .frame {
      padding: 0 var(--s3);
    }
    main {
      padding: var(--s5) 0 var(--s6);
    }
  }
</style>
