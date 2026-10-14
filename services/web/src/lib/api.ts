// Typed client for the Shorty API (same origin, routed by the gateway).

export interface Link {
  slug: string;
  url: string;
  custom: boolean;
  created_at: string;
  short_url: string;
}

export interface LinkList {
  links: Link[];
  next_before?: string;
}

export interface Point {
  t: string;
  clicks: number;
}

export interface Stats {
  slug: string;
  total: number;
  last_click_at: string | null;
  hourly: Point[];
  daily: Point[];
  referrers: { referrer: string; clicks: number }[];
}

export class ApiError extends Error {
  constructor(
    public status: number,
    public code: string,
    message: string
  ) {
    super(message);
  }
}

type Fetch = typeof fetch;

async function request<T>(f: Fetch, path: string, init?: RequestInit): Promise<T> {
  let res: Response;
  try {
    res = await f(path, {
      ...init,
      headers: { Accept: 'application/json', ...(init?.body ? { 'Content-Type': 'application/json' } : {}) }
    });
  } catch {
    throw new ApiError(0, 'network', 'Network error. Check your connection and try again.');
  }
  if (!res.ok) {
    let code = 'http_' + res.status;
    let message = `Request failed (${res.status}).`;
    try {
      const body = await res.json();
      code = body.code ?? code;
      message = body.error ?? message;
    } catch {
      /* non-JSON error body */
    }
    throw new ApiError(res.status, code, message);
  }
  return (await res.json()) as T;
}

export const createLink = (f: Fetch, url: string, slug?: string) =>
  request<Link>(f, '/api/links', { method: 'POST', body: JSON.stringify(slug ? { url, slug } : { url }) });

export const listLinks = (f: Fetch, limit = 20, before?: string) =>
  request<LinkList>(f, `/api/links?limit=${limit}${before ? '&before=' + encodeURIComponent(before) : ''}`);

export const getLink = (f: Fetch, slug: string) => request<Link>(f, `/api/links/${encodeURIComponent(slug)}`);

export const getStats = (f: Fetch, slug: string) => request<Stats>(f, `/api/stats/${encodeURIComponent(slug)}`);
