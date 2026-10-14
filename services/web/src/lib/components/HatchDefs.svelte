<!-- Shared SVG patterns. Hatching replaces grey: density conveys emphasis. -->
<svg class="defs" aria-hidden="true" focusable="false" width="0" height="0">
  <defs>
    <pattern id="hatch" width="6" height="6" patternUnits="userSpaceOnUse" patternTransform="rotate(45)">
      <line x1="0" y1="0" x2="0" y2="6" stroke="currentColor" stroke-width="1.5" />
    </pattern>
    <pattern id="hatch-sparse" width="8" height="8" patternUnits="userSpaceOnUse" patternTransform="rotate(45)">
      <line x1="0" y1="0" x2="0" y2="8" stroke="currentColor" stroke-width="1" />
    </pattern>
    <pattern id="dots" width="6" height="6" patternUnits="userSpaceOnUse">
      <rect x="2" y="2" width="1.5" height="1.5" fill="currentColor" />
    </pattern>
  </defs>
</svg>

<style>
  .defs {
    position: absolute;
    width: 0;
    height: 0;
    overflow: hidden;
    color: var(--fg);
  }
</style>
