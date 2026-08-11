export interface FieldViolation {
  field: string;
  rule: string;
  message: string;
}

export interface ProblemDetails {
  type: string;
  title: string;
  status: number;
  detail: string;
  instance?: string;
  trace_id?: string;
  errors?: FieldViolation[];
}

export type CredentialSupplier = () =>
  | HeadersInit
  | undefined
  | Promise<HeadersInit | undefined>;

export interface TransportOptions {
  baseURL: string;
  fetch?: typeof globalThis.fetch;
  timeoutMs?: number;
  credentials?: CredentialSupplier;
  onUnauthorized?: (error: TransportError) => void | Promise<void>;
}

export class TransportError extends Error {
  readonly status?: number;
  readonly problem?: ProblemDetails;
  readonly requestID?: string;

  constructor(
    message: string,
    options: {
      status?: number;
      problem?: ProblemDetails;
      requestID?: string;
      cause?: unknown;
    } = {},
  ) {
    super(message, { cause: options.cause });
    this.name = "TransportError";
    this.status = options.status;
    this.problem = options.problem;
    this.requestID = options.requestID;
  }
}

export class HTTPTransport {
  readonly #baseURL: URL;
  readonly #fetch: typeof globalThis.fetch;
  readonly #timeoutMs: number;
  readonly #credentials?: CredentialSupplier;
  readonly #onUnauthorized?: TransportOptions["onUnauthorized"];

  constructor(options: TransportOptions) {
    if (!options.baseURL) throw new Error("HTTPTransport requires a baseURL");
    if (options.timeoutMs !== undefined && options.timeoutMs <= 0) {
      throw new Error("HTTPTransport timeoutMs must be positive");
    }
    this.#baseURL = new URL(options.baseURL);
    this.#fetch = options.fetch ?? globalThis.fetch.bind(globalThis);
    this.#timeoutMs = options.timeoutMs ?? 15_000;
    this.#credentials = options.credentials;
    this.#onUnauthorized = options.onUnauthorized;
  }

  async json<T>(
    method: string,
    path: string,
    body?: unknown,
    init: Omit<RequestInit, "method" | "body"> = {},
  ): Promise<T | undefined> {
    const headers = new Headers(init.headers);
    headers.set("Accept", "application/json, application/problem+json");
    let encodedBody: BodyInit | undefined;
    if (body !== undefined) {
      headers.set("Content-Type", "application/json");
      encodedBody = JSON.stringify(body);
    }
    return this.request<T>(path, { ...init, method, headers, body: encodedBody });
  }

  async request<T>(path: string, init: RequestInit = {}): Promise<T | undefined> {
    const headers = new Headers(init.headers);
    const supplied = await this.#credentials?.();
    if (supplied) new Headers(supplied).forEach((value, key) => headers.set(key, value));
    if (init.signal?.aborted) throw new TransportError("Request aborted");

    const controller = new AbortController();
    const timeout = setTimeout(() => controller.abort("timeout"), this.#timeoutMs);
    const abort = () => controller.abort(init.signal?.reason);
    init.signal?.addEventListener("abort", abort, { once: true });
    let response: Response;
    try {
      response = await this.#fetch(new URL(path, this.#baseURL), {
        ...init,
        headers,
        signal: controller.signal,
      });
    } catch (cause) {
      if (init.signal?.aborted) throw new TransportError("Request aborted", { cause });
      if (controller.signal.aborted) throw new TransportError("Request timed out", { cause });
      throw new TransportError("Network request failed", { cause });
    } finally {
      clearTimeout(timeout);
      init.signal?.removeEventListener("abort", abort);
    }

    const requestID = response.headers.get("x-request-id") ?? response.headers.get("x-trace-id") ?? undefined;
    if (!response.ok) {
      const problem = await decodeProblem(response);
      const error = new TransportError(problem?.title ?? `HTTP request failed (${response.status})`, {
        status: response.status,
        problem,
        requestID,
      });
      if (response.status === 401) await this.#onUnauthorized?.(error);
      throw error;
    }
    if (response.status === 204 || response.status === 205) return undefined;
    const text = await response.text();
    if (!text.trim()) return undefined;
    if (!isJSON(response.headers.get("content-type"))) {
      throw new TransportError("Expected a JSON response", { status: response.status, requestID });
    }
    try {
      return JSON.parse(text) as T;
    } catch (cause) {
      throw new TransportError("Response contained malformed JSON", { status: response.status, requestID, cause });
    }
  }
}

async function decodeProblem(response: Response): Promise<ProblemDetails | undefined> {
  if (!isJSON(response.headers.get("content-type"))) return undefined;
  const text = await response.text();
  if (!text.trim()) return undefined;
  try {
    const value = JSON.parse(text) as Partial<ProblemDetails>;
    if (
      typeof value.type === "string" &&
      typeof value.title === "string" &&
      typeof value.status === "number" &&
      typeof value.detail === "string"
    ) return value as ProblemDetails;
  } catch {
    // The surrounding HTTP status still represents a malformed error body.
  }
  return undefined;
}

function isJSON(contentType: string | null): boolean {
  const mediaType = contentType?.split(";", 1)[0]?.trim().toLowerCase();
  return mediaType === "application/json" || mediaType === "application/problem+json" || mediaType?.endsWith("+json") === true;
}
