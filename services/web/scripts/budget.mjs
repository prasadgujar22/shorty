// Fails the build when gzipped JS+CSS exceeds the budget (fonts excluded:
// they are separate, cacheable, and not render-blocking).
import { readdirSync, readFileSync, statSync } from 'node:fs';
import { join } from 'node:path';
import { gzipSync } from 'node:zlib';

const BUDGET = 100 * 1024;
const dir = 'build';
const files = [];
(function walk(d) {
  for (const f of readdirSync(d)) {
    const p = join(d, f);
    if (statSync(p).isDirectory()) walk(p);
    else if (/\.(js|css)$/.test(f)) files.push(p);
  }
})(dir);

let total = 0;
for (const f of files) {
  const gz = gzipSync(readFileSync(f), { level: 9 }).length;
  total += gz;
  console.log(`${(gz / 1024).toFixed(1).padStart(7)} KB  ${f}`);
}
// The inline theme script in index.html counts too.
const inline = (readFileSync(join(dir, 'index.html'), 'utf8').match(/<script>[\s\S]*?<\/script>/g) ?? []).join('');
total += gzipSync(inline).length;
console.log(`${(total / 1024).toFixed(1).padStart(7)} KB  TOTAL gzipped JS+CSS (budget ${BUDGET / 1024} KB)`);
if (total > BUDGET) {
  console.error('Bundle budget exceeded');
  process.exit(1);
}
