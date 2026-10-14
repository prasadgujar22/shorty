// Generates the Grafana dashboards in this directory (run: make dashboards).
// One RED dashboard per service plus a Shorty overview, built from shared
// panel helpers so every dashboard reads the same way.
import { writeFileSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';

const here = dirname(fileURLToPath(import.meta.url));
const DS = { type: 'prometheus', uid: '${datasource}' };
const LOKI = { type: 'loki', uid: 'loki' };
const NS = 'namespace=~"$namespace"';

let nextId = 1;
const resetIds = () => (nextId = 1);

const target = (expr, legend = '', extra = {}) => ({ datasource: DS, expr, legendFormat: legend, refId: String.fromCharCode(64 + nextId % 26 + 1), ...extra });

function panel(type, title, gridPos, targets, opts = {}) {
  return {
    id: nextId++,
    type,
    title,
    datasource: DS,
    gridPos,
    targets: targets.map((t, i) => ({ ...t, refId: String.fromCharCode(65 + i) })),
    fieldConfig: { defaults: { unit: opts.unit ?? 'short', min: opts.min, max: opts.max, decimals: opts.decimals, thresholds: opts.thresholds, custom: opts.custom ?? {} }, overrides: opts.overrides ?? [] },
    options: opts.options ?? {},
    description: opts.description ?? ''
  };
}

const ts = (title, pos, targets, opts = {}) =>
  panel('timeseries', title, pos, targets, {
    ...opts,
    custom: { lineWidth: 1, fillOpacity: 0, showPoints: 'never', spanNulls: true, ...(opts.stack ? { stacking: { mode: 'normal' }, fillOpacity: 20 } : {}) },
    options: { legend: { displayMode: 'table', placement: 'right', calcs: ['mean', 'max', 'lastNotNull'] }, tooltip: { mode: 'multi', sort: 'desc' } }
  });

const stat = (title, pos, expr, opts = {}) =>
  panel('stat', title, pos, [target(expr, opts.legend ?? '')], {
    ...opts,
    options: { reduceOptions: { calcs: ['lastNotNull'] }, colorMode: 'value', graphMode: 'area', textMode: 'value' }
  });

const row = (title, y) => ({ id: nextId++, type: 'row', title, collapsed: false, gridPos: { h: 1, w: 24, x: 0, y }, panels: [] });

const logs = (title, pos, expr) => ({
  id: nextId++,
  type: 'logs',
  title,
  datasource: LOKI,
  gridPos: pos,
  targets: [{ datasource: LOKI, expr, refId: 'A' }],
  options: { showTime: true, wrapLogMessage: true, sortOrder: 'Descending', enableLogDetails: true, prettifyLogMessage: false }
});

const thresholdsErr = { mode: 'absolute', steps: [{ color: 'green', value: null }, { color: 'orange', value: 0.001 }, { color: 'red', value: 0.01 }] };
const thresholdsLat = { mode: 'absolute', steps: [{ color: 'green', value: null }, { color: 'orange', value: 0.05 }, { color: 'red', value: 0.25 }] };

function dashboard(uid, title, panels, tags = []) {
  return {
    uid,
    title,
    tags: ['shorty', ...tags],
    timezone: 'browser',
    editable: true,
    graphTooltip: 1,
    refresh: '10s',
    schemaVersion: 39,
    time: { from: 'now-30m', to: 'now' },
    templating: {
      list: [
        { name: 'datasource', type: 'datasource', query: 'prometheus', current: { text: 'Prometheus', value: 'prometheus' }, hide: 0 },
        {
          name: 'namespace',
          type: 'query',
          datasource: DS,
          query: { query: 'label_values(shorty_build_info, namespace)', refId: 'ns' },
          definition: 'label_values(shorty_build_info, namespace)',
          includeAll: true,
          multi: true,
          current: { text: 'All', value: '$__all' },
          refresh: 2
        }
      ]
    },
    annotations: {
      list: [
        {
          name: 'Deploys',
          datasource: DS,
          enable: true,
          iconColor: 'blue',
          expr: `changes(shorty_build_info{${NS}}[1m]) > 0`,
          titleFormat: 'Deploy {{job}} {{version}}',
          step: '30s'
        }
      ]
    },
    panels
  };
}

/** RED section for one Go service. Returns panels laid out from y. */
function red(job, y) {
  const sel = `job="${job}", ${NS}`;
  return [
    row(`${job} · RED`, y),
    stat('Request rate', { h: 4, w: 6, x: 0, y: y + 1 }, `sum(rate(shorty_http_requests_total{${sel}}[$__rate_interval]))`, { unit: 'reqps', decimals: 1 }),
    stat('Error ratio (5xx)', { h: 4, w: 6, x: 6, y: y + 1 },
      `sum(rate(shorty_http_requests_total{${sel}, code=~"5.."}[$__rate_interval])) / clamp_min(sum(rate(shorty_http_requests_total{${sel}}[$__rate_interval])), 1e-9)`,
      { unit: 'percentunit', decimals: 3, thresholds: thresholdsErr }),
    stat('p99 latency', { h: 4, w: 6, x: 12, y: y + 1 },
      `histogram_quantile(0.99, sum by (le) (rate(shorty_http_request_duration_seconds_bucket{${sel}}[$__rate_interval])))`,
      { unit: 's', decimals: 4, thresholds: thresholdsLat }),
    stat('Ready pods', { h: 4, w: 6, x: 18, y: y + 1 }, `sum(kube_pod_status_ready{${NS}, condition="true", pod=~"${job}-.*"})`, { decimals: 0 }),
    ts('Rate by route and code', { h: 8, w: 12, x: 0, y: y + 5 }, [
      target(`sum by (route, code) (rate(shorty_http_requests_total{${sel}}[$__rate_interval]))`, '{{route}} {{code}}')
    ], { unit: 'reqps' }),
    ts('Latency p50 / p95 / p99', { h: 8, w: 12, x: 12, y: y + 5 }, [0.5, 0.95, 0.99].map((q) =>
      target(`histogram_quantile(${q}, sum by (le) (rate(shorty_http_request_duration_seconds_bucket{${sel}}[$__rate_interval])))`, `p${q * 100}`)
    ), { unit: 's' }),
    ts('Errors (non-2xx/3xx) by route', { h: 7, w: 12, x: 0, y: y + 13 }, [
      target(`sum by (route, code) (rate(shorty_http_requests_total{${sel}, code!~"[23].."}[$__rate_interval]))`, '{{route}} {{code}}')
    ], { unit: 'reqps' }),
    ts('In-flight requests', { h: 7, w: 12, x: 12, y: y + 13 }, [target(`sum by (pod) (shorty_http_requests_in_flight{${sel}})`, '{{pod}}')])
  ];
}

/** Saturation / Go runtime section. */
function runtime(job, y) {
  const sel = `job="${job}", ${NS}`;
  const c = `${NS}, pod=~"${job}-.*", container="${job}"`;
  return [
    row(`${job} · saturation`, y),
    ts('CPU (cores) vs limit', { h: 7, w: 8, x: 0, y: y + 1 }, [
      target(`sum by (pod) (rate(container_cpu_usage_seconds_total{${c}}[$__rate_interval]))`, '{{pod}}'),
      target(`max(kube_pod_container_resource_limits{${NS}, container="${job}", resource="cpu"})`, 'limit')
    ]),
    ts('CPU throttling', { h: 7, w: 8, x: 8, y: y + 1 }, [
      target(`sum by (pod) (rate(container_cpu_cfs_throttled_periods_total{${c}}[$__rate_interval])) / clamp_min(sum by (pod) (rate(container_cpu_cfs_periods_total{${c}}[$__rate_interval])), 1e-9)`, '{{pod}}')
    ], { unit: 'percentunit' }),
    ts('Memory working set vs GOMEMLIMIT', { h: 7, w: 8, x: 16, y: y + 1 }, [
      target(`sum by (pod) (container_memory_working_set_bytes{${c}})`, '{{pod}}'),
      target(`max(go_gc_gomemlimit_bytes{${sel}})`, 'GOMEMLIMIT')
    ], { unit: 'bytes' }),
    ts('Goroutines', { h: 7, w: 8, x: 0, y: y + 8 }, [target(`go_goroutines{${sel}}`, '{{pod}}')]),
    ts('GC cycles / s', { h: 7, w: 8, x: 8, y: y + 8 }, [target(`rate(go_gc_duration_seconds_count{${sel}}[$__rate_interval])`, "{{pod}}")]),
    ts('Restarts', { h: 7, w: 8, x: 16, y: y + 8 }, [target(`sum by (pod) (increase(kube_pod_container_status_restarts_total{${NS}, container="${job}"}[$__rate_interval]))`, '{{pod}}')])
  ];
}

function dbPool(job, y) {
  const sel = `job="${job}", ${NS}`;
  return [
    row(`${job} · Postgres pool`, y),
    ts('Connections (acquired / idle / max)', { h: 7, w: 12, x: 0, y: y + 1 }, [
      target(`sum(shorty_db_pool_acquired_connections{${sel}})`, 'acquired'),
      target(`sum(shorty_db_pool_idle_connections{${sel}})`, 'idle'),
      target(`sum(shorty_db_pool_max_connections{${sel}})`, 'max')
    ]),
    ts('Acquire wait (avg) and empty-pool acquires', { h: 7, w: 12, x: 12, y: y + 1 }, [
      target(`sum(rate(shorty_db_pool_acquire_seconds_total{${sel}}[$__rate_interval])) / clamp_min(sum(rate(shorty_db_pool_acquires_total{${sel}}[$__rate_interval])), 1e-9)`, 'avg wait (s)'),
      target(`sum(rate(shorty_db_pool_empty_acquires_total{${sel}}[$__rate_interval]))`, 'empty acquires/s')
    ])
  ];
}

const logPanel = (app, y) => [row('Logs', y), logs(`${app} warnings and errors`, { h: 10, w: 24, x: 0, y: y + 1 }, `{namespace=~"$namespace", app="${app}", level=~"WARN|ERROR"}`)];

const out = {};

// ---------------- links-svc ----------------
resetIds();
out['links-svc'] = dashboard('shorty-links-svc', 'Shorty / links-svc', [
  ...red('links-svc', 0),
  row('links-svc · business', 20),
  ts('Links created', { h: 7, w: 24, x: 0, y: 21 }, [target(`sum by (kind) (rate(shorty_links_created_total{job="links-svc", ${NS}}[$__rate_interval]))`, '{{kind}}')], { unit: 'ops' }),
  ...dbPool('links-svc', 28),
  ...runtime('links-svc', 36),
  ...logPanel('links-svc', 51)
], ['links-svc']);

// ---------------- redirect-svc ----------------
resetIds();
const r = `job="redirect-svc", ${NS}`;
out['redirect-svc'] = dashboard('shorty-redirect-svc', 'Shorty / redirect-svc', [
  ...red('redirect-svc', 0),
  row('redirect-svc · cache (Redis cache-aside)', 20),
  stat('Cache hit ratio', { h: 7, w: 6, x: 0, y: 21 },
    `sum(rate(shorty_redirect_cache_requests_total{${r}, result=~"hit|negative_hit"}[$__rate_interval])) / clamp_min(sum(rate(shorty_redirect_cache_requests_total{${r}}[$__rate_interval])), 1e-9)`,
    { unit: 'percentunit', decimals: 1 }),
  ts('Cache results', { h: 7, w: 9, x: 6, y: 21 }, [target(`sum by (result) (rate(shorty_redirect_cache_requests_total{${r}}[$__rate_interval]))`, '{{result}}')], { unit: 'ops' }),
  ts('Lookup latency p99 (cache vs links-svc) + breaker', { h: 7, w: 9, x: 15, y: 21 }, [
    target(`histogram_quantile(0.99, sum by (le, source) (rate(shorty_redirect_lookup_seconds_bucket{${r}}[$__rate_interval])))`, 'p99 {{source}}'),
    target(`max(shorty_redis_breaker_open{${r}})`, 'breaker open')
  ], { unit: 's' }),
  row('redirect-svc · click events → JetStream', 28),
  ts('Clicks enqueued / published / dropped', { h: 7, w: 8, x: 0, y: 29 }, [
    target(`sum(rate(shorty_clicks_enqueued_total{${r}}[$__rate_interval]))`, 'enqueued'),
    target(`sum(rate(shorty_clicks_published_total{${r}}[$__rate_interval]))`, 'published'),
    target(`sum(rate(shorty_clicks_dropped_total{${r}}[$__rate_interval]))`, 'dropped'),
    target(`sum(rate(shorty_clicks_publish_retries_total{${r}}[$__rate_interval]))`, 'retries')
  ], { unit: 'ops' }),
  ts('Buffer depth (per pod) vs capacity', { h: 7, w: 8, x: 8, y: 29 }, [
    target(`shorty_clicks_buffer_depth{${r}}`, '{{pod}}'),
    target(`max(shorty_clicks_buffer_capacity{${r}})`, 'capacity')
  ]),
  ts('Publish-to-ack latency p50 / p99', { h: 7, w: 8, x: 16, y: 29 }, [0.5, 0.99].map((q) =>
    target(`histogram_quantile(${q}, sum by (le) (rate(shorty_clicks_publish_seconds_bucket{${r}}[$__rate_interval])))`, `p${q * 100}`)
  ), { unit: 's' }),
  row('redirect-svc · autoscaling', 36),
  ts('Replicas (HPA) and per-pod RPS', { h: 7, w: 24, x: 0, y: 37 }, [
    target(`max(kube_horizontalpodautoscaler_status_current_replicas{${NS}, horizontalpodautoscaler="redirect-svc"})`, 'current replicas'),
    target(`max(kube_horizontalpodautoscaler_status_desired_replicas{${NS}, horizontalpodautoscaler="redirect-svc"})`, 'desired replicas'),
    target(`sum by (pod) (rate(shorty_http_requests_total{${r}}[1m]))`, 'rps {{pod}}')
  ]),
  ...runtime('redirect-svc', 44),
  ...logPanel('redirect-svc', 59)
], ['redirect-svc']);

// ---------------- analytics-svc ----------------
resetIds();
const a = `job="analytics-svc", ${NS}`;
out['analytics-svc'] = dashboard('shorty-analytics-svc', 'Shorty / analytics-svc', [
  ...red('analytics-svc', 0),
  row('analytics-svc · JetStream consumer', 20),
  ts('Events by result (processed / duplicate / invalid)', { h: 7, w: 12, x: 0, y: 21 }, [
    target(`sum by (result) (rate(shorty_analytics_events_total{${a}}[$__rate_interval]))`, '{{result}}'),
    target(`sum(rate(shorty_analytics_redelivered_total{${a}}[$__rate_interval]))`, 'redelivered')
  ], { unit: 'ops' }),
  ts('Consumer lag (pending + ack pending)', { h: 7, w: 12, x: 12, y: 21 }, [
    target(`max(shorty_analytics_consumer_pending{${a}})`, 'pending'),
    target(`max(shorty_analytics_consumer_ack_pending{${a}})`, 'ack pending')
  ]),
  ts('End-to-end freshness (newest event age)', { h: 7, w: 8, x: 0, y: 28 }, [target(`max by (pod) (shorty_analytics_last_event_age_seconds{${a}})`, '{{pod}}')], { unit: 's' }),
  ts('Batch duration p50 / p99', { h: 7, w: 8, x: 8, y: 28 }, [0.5, 0.99].map((q) =>
    target(`histogram_quantile(${q}, sum by (le) (rate(shorty_analytics_batch_seconds_bucket{${a}}[$__rate_interval])))`, `p${q * 100}`)
  ), { unit: 's' }),
  ts('Batch size (avg) and apply errors', { h: 7, w: 8, x: 16, y: 28 }, [
    target(`sum(rate(shorty_analytics_batch_size_sum{${a}}[$__rate_interval])) / clamp_min(sum(rate(shorty_analytics_batch_size_count{${a}}[$__rate_interval])), 1e-9)`, 'avg batch'),
    target(`sum(rate(shorty_analytics_apply_errors_total{${a}}[$__rate_interval]))`, 'apply errors/s')
  ]),
  ...dbPool('analytics-svc', 35),
  ...runtime('analytics-svc', 43),
  ...logPanel('analytics-svc', 58)
], ['analytics-svc']);

// ---------------- web (Traefik RED + Nginx) ----------------
resetIds();
const tw = `service=~"($namespace)-web-.*"`;
out.web = dashboard('shorty-web', 'Shorty / web', [
  row('web · RED (measured at the gateway)', 0),
  stat('Request rate', { h: 4, w: 8, x: 0, y: 1 }, `sum(rate(traefik_service_requests_total{${tw}}[$__rate_interval]))`, { unit: 'reqps', decimals: 1 }),
  stat('Error ratio (5xx)', { h: 4, w: 8, x: 8, y: 1 },
    `sum(rate(traefik_service_requests_total{${tw}, code=~"5.."}[$__rate_interval])) / clamp_min(sum(rate(traefik_service_requests_total{${tw}}[$__rate_interval])), 1e-9)`,
    { unit: 'percentunit', decimals: 3, thresholds: thresholdsErr }),
  stat('p99 latency', { h: 4, w: 8, x: 16, y: 1 },
    `histogram_quantile(0.99, sum by (le) (rate(traefik_service_request_duration_seconds_bucket{${tw}}[$__rate_interval])))`,
    { unit: 's', decimals: 4, thresholds: thresholdsLat }),
  ts('Requests by code', { h: 8, w: 12, x: 0, y: 5 }, [target(`sum by (code) (rate(traefik_service_requests_total{${tw}}[$__rate_interval]))`, '{{code}}')], { unit: 'reqps' }),
  ts('Latency p50 / p95 / p99', { h: 8, w: 12, x: 12, y: 5 }, [0.5, 0.95, 0.99].map((q) =>
    target(`histogram_quantile(${q}, sum by (le) (rate(traefik_service_request_duration_seconds_bucket{${tw}}[$__rate_interval])))`, `p${q * 100}`)
  ), { unit: 's' }),
  row('web · Nginx', 13),
  ts('Nginx connections', { h: 7, w: 12, x: 0, y: 14 }, [
    target(`sum by (pod) (nginx_connections_active{job="web", ${NS}})`, 'active {{pod}}'),
    target(`sum by (pod) (nginx_connections_waiting{job="web", ${NS}})`, 'waiting {{pod}}')
  ]),
  ts('Nginx requests/s', { h: 7, w: 12, x: 12, y: 14 }, [target(`sum by (pod) (rate(nginx_http_requests_total{job="web", ${NS}}[$__rate_interval]))`, '{{pod}}')], { unit: 'reqps' }),
  ...logPanel('web', 21)
], ['web']);

// ---------------- overview ----------------
resetIds();
const svcs = ['redirect-svc', 'links-svc', 'analytics-svc'];
const overview = [row('Golden signals per service', 0)];
svcs.forEach((job, i) => {
  const sel = `job="${job}", ${NS}`;
  const y = 1 + i * 4;
  overview.push(
    stat(`${job} · rps`, { h: 4, w: 6, x: 0, y }, `sum(rate(shorty_http_requests_total{${sel}}[$__rate_interval]))`, { unit: 'reqps', decimals: 1 }),
    stat(`${job} · 5xx`, { h: 4, w: 6, x: 6, y }, `sum(rate(shorty_http_requests_total{${sel}, code=~"5.."}[$__rate_interval])) / clamp_min(sum(rate(shorty_http_requests_total{${sel}}[$__rate_interval])), 1e-9)`, { unit: 'percentunit', decimals: 3, thresholds: thresholdsErr }),
    stat(`${job} · p99`, { h: 4, w: 6, x: 12, y }, `histogram_quantile(0.99, sum by (le) (rate(shorty_http_request_duration_seconds_bucket{${sel}}[$__rate_interval])))`, { unit: 's', decimals: 4, thresholds: thresholdsLat }),
    stat(`${job} · ready pods`, { h: 4, w: 6, x: 18, y }, `sum(kube_pod_status_ready{${NS}, condition="true", pod=~"${job}-.*"})`, { decimals: 0 })
  );
});
overview.push(
  row('Gateway (Traefik)', 13),
  ts('Requests by backend', { h: 8, w: 12, x: 0, y: 14 }, [target(`sum by (service) (rate(traefik_service_requests_total{service=~"($namespace)-.*"}[$__rate_interval]))`, '{{service}}')], { unit: 'reqps' }),
  ts('Gateway 5xx by backend', { h: 8, w: 12, x: 12, y: 14 }, [target(`sum by (service) (rate(traefik_service_requests_total{service=~"($namespace)-.*", code=~"5.."}[$__rate_interval]))`, '{{service}}')], { unit: 'reqps' }),
  row('Click pipeline (redirect → JetStream → analytics)', 22),
  ts('Clicks/s at each stage', { h: 8, w: 12, x: 0, y: 23 }, [
    target(`sum(rate(shorty_clicks_enqueued_total{${NS}}[$__rate_interval]))`, '1 enqueued (redirect)'),
    target(`sum(rate(shorty_clicks_published_total{${NS}}[$__rate_interval]))`, '2 acked by JetStream'),
    target(`sum(rate(shorty_analytics_events_total{${NS}, result="processed"}[$__rate_interval]))`, '3 aggregated (analytics)'),
    target(`sum(rate(shorty_clicks_dropped_total{${NS}}[$__rate_interval]))`, 'dropped')
  ], { unit: 'ops' }),
  ts('Backlog: redirect buffers + consumer lag; freshness', { h: 8, w: 12, x: 12, y: 23 }, [
    target(`sum(shorty_clicks_buffer_depth{${NS}})`, 'redirect buffers'),
    target(`max(shorty_analytics_consumer_pending{${NS}}) + max(shorty_analytics_consumer_ack_pending{${NS}})`, 'consumer lag'),
    target(`max(shorty_analytics_last_event_age_seconds{${NS}})`, 'freshness (s)')
  ]),
  row('Data tier', 31),
  ts('Postgres: backends and replication lag', { h: 7, w: 8, x: 0, y: 32 }, [
    target(`sum by (pod) (cnpg_backends_total{${NS}})`, 'backends {{pod}}'),
    target(`max by (pod) (cnpg_pg_replication_lag{${NS}})`, 'repl lag s {{pod}}')
  ]),
  ts('Redis: hit rate and memory', { h: 7, w: 8, x: 8, y: 32 }, [
    target(`sum(rate(redis_keyspace_hits_total{${NS}}[$__rate_interval])) / clamp_min(sum(rate(redis_keyspace_hits_total{${NS}}[$__rate_interval])) + sum(rate(redis_keyspace_misses_total{${NS}}[$__rate_interval])), 1e-9)`, 'hit ratio'),
    target(`sum by (pod) (redis_memory_used_bytes{${NS}}) / 1048576`, 'MiB {{pod}}')
  ]),
  ts('NATS: JetStream messages / bytes', { h: 7, w: 8, x: 16, y: 32 }, [
    target(`sum by (pod) (nats_varz_jetstream_stats_messages{${NS}})`, 'msgs {{pod}}'),
    target(`sum by (pod) (nats_varz_jetstream_stats_storage{${NS}}) / 1048576`, 'MiB {{pod}}')
  ]),
  row('Cluster', 39),
  ts('Pod restarts (Shorty namespaces)', { h: 7, w: 12, x: 0, y: 40 }, [target(`sum by (pod) (increase(kube_pod_container_status_restarts_total{${NS}}[5m])) > 0`, '{{pod}}')]),
  ts('Pods per node (Shorty)', { h: 7, w: 12, x: 12, y: 40 }, [target(`count by (node) (kube_pod_info{${NS}})`, '{{node}}')]),
  logs('Errors across Shorty', { h: 10, w: 24, x: 0, y: 47 }, `{namespace=~"$namespace", level="ERROR"}`)
);
out.overview = dashboard('shorty-overview', 'Shorty / Overview', overview, ['overview']);

for (const [name, d] of Object.entries(out)) {
  writeFileSync(join(here, `${name}.json`), JSON.stringify(d, null, 2) + '\n');
  console.log(`wrote ${name}.json (${d.panels.length} panels)`);
}
