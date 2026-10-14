// k6 load test for the redirect hot path.
//
//   k6 run load/redirect.js                                   # defaults
//   k6 run -e BASE_URL=http://localhost:8088 -e HOST=ha.localhost \
//          -e RATE=800 -e DURATION=3m -e LINKS=200 load/redirect.js
//
// setup() creates LINKS fresh short links with a unique run prefix, so the
// number of 302s k6 saw can be reconciled exactly against analytics-svc
// (chaos/run.sh does that). Redirects are not followed: we measure Shorty,
// not example.com.
import http from 'k6/http';
import { check } from 'k6';
import { Counter, Rate } from 'k6/metrics';

const BASE = __ENV.BASE_URL || 'http://localhost:8088';
const HOST = __ENV.HOST || '';
const RATE = Number(__ENV.RATE || 500);
const DURATION = __ENV.DURATION || '2m';
const LINKS = Number(__ENV.LINKS || 100);
const RUN_ID = __ENV.RUN_ID || `k6${Date.now().toString(36)}`;
const HOT_SHARE = Number(__ENV.HOT_SHARE || 0.2); // share of traffic to the first 5% of links (Zipf-ish)
const MISS_SHARE = Number(__ENV.MISS_SHARE || 0.0); // share of requests to unknown slugs (404 path)

const headers = HOST ? { Host: HOST } : {};
const redirects = new Counter('shorty_redirects_302');
const unexpected = new Rate('shorty_unexpected_status');

// RAMP="300:30s,2400:60s,2400:120s,300:30s" switches to a ramping arrival
// rate (target rps:duration per stage), used for the HPA experiment.
const RAMP = __ENV.RAMP || '';
const stages = RAMP ? RAMP.split(',').map((s) => ({ target: Number(s.split(':')[0]), duration: s.split(':')[1] })) : [];
const peak = stages.length ? Math.max(...stages.map((s) => s.target)) : RATE;

export const options = {
  discardResponseBodies: true,
  scenarios: {
    redirect: RAMP
      ? {
          executor: 'ramping-arrival-rate',
          startRate: stages[0].target,
          timeUnit: '1s',
          stages,
          preAllocatedVUs: Math.max(50, Math.ceil(peak / 10)),
          maxVUs: Math.max(200, peak),
          gracefulStop: '10s'
        }
      : {
          executor: 'constant-arrival-rate',
          rate: RATE,
          timeUnit: '1s',
          duration: DURATION,
          preAllocatedVUs: Math.max(50, Math.ceil(RATE / 10)),
          maxVUs: Math.max(200, RATE),
          gracefulStop: '10s'
        }
  },
  summaryTrendStats: ['avg', 'min', 'med', 'p(90)', 'p(95)', 'p(99)', 'max'],
  thresholds: {
    // Informational SLOs; chaos runs are expected to breach some of them.
    'http_req_duration{name:redirect}': ['p(95)<50', 'p(99)<200'],
    shorty_unexpected_status: ['rate<0.01']
  },
  setupTimeout: '2m',
  noConnectionReuse: false,
  userAgent: 'shorty-k6/1.0'
};

export function setup() {
  const slugs = [];
  for (let i = 0; i < LINKS; i++) {
    const slug = `${RUN_ID}-${i}`;
    const res = http.post(`${BASE}/api/links`, JSON.stringify({ url: `https://example.com/load/${RUN_ID}/${i}`, slug }), {
      headers: { ...headers, 'Content-Type': 'application/json' },
      responseType: 'none'
    });
    if (res.status !== 201) throw new Error(`setup: create ${slug} -> ${res.status}`);
    slugs.push(slug);
  }
  // Warm nothing on purpose: the first requests exercise cache misses.
  return { slugs, runId: RUN_ID };
}

export default function (data) {
  const { slugs } = data;
  let slug;
  const r = Math.random();
  if (r < MISS_SHARE) {
    slug = `${RUN_ID}-missing-${Math.floor(Math.random() * 1000)}`;
  } else if (r < MISS_SHARE + HOT_SHARE) {
    slug = slugs[Math.floor(Math.random() * Math.max(1, Math.floor(slugs.length * 0.05)))];
  } else {
    slug = slugs[Math.floor(Math.random() * slugs.length)];
  }
  const res = http.get(`${BASE}/${slug}`, {
    headers: { ...headers, Referer: 'https://k6.io/load' },
    redirects: 0,
    tags: { name: 'redirect' },
    timeout: '5s'
  });
  const expected = slug.includes('-missing-') ? 404 : 302;
  if (res.status === 302) redirects.add(1);
  unexpected.add(res.status !== expected);
  check(res, { 'expected status': (x) => x.status === expected });
}

export function handleSummary(data) {
  const d = data.metrics['http_req_duration{name:redirect}'] || data.metrics.http_req_duration;
  const reqs = data.metrics.http_reqs;
  const out = {
    run_id: RUN_ID,
    rate_target: RAMP ? `ramp ${RAMP}` : RATE,
    duration: RAMP ? 'ramp' : DURATION,
    requests: reqs ? reqs.values.count : 0,
    rps: reqs ? Math.round(reqs.values.rate) : 0,
    redirects_302: data.metrics.shorty_redirects_302 ? data.metrics.shorty_redirects_302.values.count : 0,
    unexpected_rate: data.metrics.shorty_unexpected_status ? data.metrics.shorty_unexpected_status.values.rate : 0,
    failed_rate: data.metrics.http_req_failed ? data.metrics.http_req_failed.values.rate : 0,
    p50_ms: d.values.med,
    p95_ms: d.values['p(95)'],
    p99_ms: d.values['p(99)'],
    max_ms: d.values.max,
    dropped_iterations: data.metrics.dropped_iterations ? data.metrics.dropped_iterations.values.count : 0
  };
  const file = __ENV.SUMMARY_FILE || `load/results/${RUN_ID}.json`;
  const line = `run ${RUN_ID}: ${out.rps} rps, p50 ${out.p50_ms.toFixed(2)}ms p95 ${out.p95_ms.toFixed(2)}ms p99 ${out.p99_ms.toFixed(2)}ms, ` +
    `unexpected ${(out.unexpected_rate * 100).toFixed(3)}%, 302s ${out.redirects_302}\n`;
  // In-cluster runs (load/tune.sh) read the JSON from the pod log.
  return { [file]: JSON.stringify(out, null, 2), stdout: line + 'SUMMARY ' + JSON.stringify(out) + '\n' };
}
