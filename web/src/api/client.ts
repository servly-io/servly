import type { ErrorBody } from './types';

/** An error response from the API, or a network failure (status 0). */
export class ApiError extends Error {
  constructor(
    readonly status: number,
    readonly code: string,
    message: string,
    readonly details?: Record<string, unknown>,
    readonly requestId?: string,
  ) {
    super(message);
    this.name = 'ApiError';
  }

  get isNetwork(): boolean {
    return this.status === 0;
  }
}

let csrfToken: string | null = null;

/** Set the CSRF token from GET /api/auth/session; sent on every state-changing request. */
export function setCsrfToken(token: string | null): void {
  csrfToken = token;
}

export interface RequestOptions {
  body?: unknown;
  /** Multipart body (file upload); the browser sets the boundary. */
  form?: FormData;
  /** Idempotency-Key header, required for POST /api/public/orders. */
  idempotencyKey?: string;
  signal?: AbortSignal;
}

/** Call the API. Resolves with the parsed JSON body, rejects with ApiError. */
export async function api<T>(method: string, path: string, opts: RequestOptions = {}): Promise<T> {
  const headers: Record<string, string> = { Accept: 'application/json' };
  if (opts.body !== undefined && !opts.form) headers['Content-Type'] = 'application/json';
  if (opts.idempotencyKey) headers['Idempotency-Key'] = opts.idempotencyKey;
  if (method !== 'GET' && method !== 'HEAD' && csrfToken) headers['X-CSRF-Token'] = csrfToken;

  let res: Response;
  try {
    res = await fetch(path, {
      method,
      headers,
      credentials: 'same-origin',
      body: opts.form ?? (opts.body === undefined ? undefined : JSON.stringify(opts.body)),
      signal: opts.signal,
    });
  } catch (err) {
    if (err instanceof DOMException && err.name === 'AbortError') throw err;
    throw new ApiError(0, 'network_error', 'The server could not be reached. Check the connection.');
  }

  if (res.status === 204) return undefined as T;
  const text = await res.text();
  const json: unknown = text ? safeParse(text) : undefined;

  if (!res.ok) {
    const e = (json as ErrorBody | undefined)?.error;
    throw new ApiError(
      res.status,
      e?.code ?? 'http_error',
      e?.message ?? `Request failed (${res.status})`,
      e?.details,
      e?.request_id,
    );
  }
  return json as T;
}

function safeParse(text: string): unknown {
  try {
    return JSON.parse(text);
  } catch {
    return undefined;
  }
}

/** A fresh idempotency key for one logical submission; reuse it on retries. */
export function newIdempotencyKey(): string {
  return crypto.randomUUID();
}
