// k6: links-svc read/write path (bypasses the redirect cache), used by
// load/tune.sh to compare Postgres pool sizes.
//   k6 run -e RATE=800 -e DURATION=60s -e WRITE_SHARE=0.1 load/links.js
import http from 'k6/http';
import { check } from 'k6';

const BASE = __ENV.BASE_URL || 'http://localhost:8088';
const HOST = __ENV.HOST || '';
const RATE = Number(__ENV.RATE || 500);
const WRITE_SHARE = Number(__ENV.WRITE_SHARE || 0.1);
const RUN_ID = __ENV.RUN_ID || `lk${Date.now().toString(36)}`;
const headers = HOST ? { Host: HOST } : {};

export const options = {
  discardResponseBodies: true,
  scenarios: {
    links: {
      executor: 'constant-arrival-rate', rate: RATE, timeUnit: '1s', duration: __ENV.DURATION || '60s',
      preAllocatedVUs: Math.max(50, RATE / 5), maxVUs: Math.max(300, RATE)
    }
  },
  summaryTrendStats: ['med', 'p(95)', 'p(99)', 'max']
};

export function setup() {
  const slugs = [];
  for (let i = 0; i < 200; i++) {
    const slug = `${RUN_ID}-${i}`;
    http.post(`${BASE}/api/links`, JSON.stringify({ url: `https://example.com/${slug}`, slug }), { headers: { ...headers, 'Content-Type': 'application/json' } });
    slugs.push(slug);
  }
  return { slugs };
}

export default function (data) {
  if (Math.random() < WRITE_SHARE) {
    const res = http.post(`${BASE}/api/links`, JSON.stringify({ url: `https://example.com/w/${Math.random()}` }), {
      headers: { ...headers, 'Content-Type': 'application/json' }, tags: { name: 'create' }
    });
    check(res, { created: (r) => r.status === 201 });
  } else {
    const slug = data.slugs[Math.floor(Math.random() * data.slugs.length)];
    const res = http.get(`${BASE}/api/links/${slug}`, { headers, tags: { name: 'get' } });
    check(res, { found: (r) => r.status === 200 });
  }
}

export function handleSummary(data) {
  const d = data.metrics.http_req_duration.values;
  const out = { rps: Math.round(data.metrics.http_reqs.values.rate), p50_ms: d.med, p95_ms: d['p(95)'], p99_ms: d['p(99)'], max_ms: d.max,
    failed_rate: data.metrics.http_req_failed ? data.metrics.http_req_failed.values.rate : 0 };
  return { [__ENV.SUMMARY_FILE || `load/results/${RUN_ID}.json`]: JSON.stringify(out), stdout: 'SUMMARY ' + JSON.stringify(out) + '\n' };
}
