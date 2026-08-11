import { describe, expect, test } from "bun:test";
import { HTTPTransport, TransportError } from "./transport";

const jsonResponse = (body: unknown, status = 200, headers: HeadersInit = {}) =>
  new Response(JSON.stringify(body), {
    status,
    headers: { "content-type": "application/json", ...headers },
  });

describe("HTTPTransport", () => {
  test("decodes JSON success and supplies credentials", async () => {
    let authorization = "";
    const transport = new HTTPTransport({
      baseURL: "https://example.test/api/",
      credentials: () => ({ authorization: "Custom credential" }),
      fetch: (async (_input, init) => {
        authorization = new Headers(init?.headers).get("authorization") ?? "";
        return jsonResponse({ ok: true });
      }) as typeof fetch,
    });
    expect(await transport.json<{ ok: boolean }>("GET", "status")).toEqual({ ok: true });
    expect(authorization).toBe("Custom credential");
  });

  test("parses Problem Details, request ID, and unauthorized callback", async () => {
    let unauthorized = 0;
    const transport = new HTTPTransport({
      baseURL: "https://example.test",
      onUnauthorized: () => { unauthorized += 1; },
      fetch: (async () => jsonResponse({
        type: "https://example.test/problems/unauthorized",
        title: "Unauthorized",
        status: 401,
        detail: "Authentication required",
      }, 401, { "x-request-id": "req-42" })) as typeof fetch,
    });
    try {
      await transport.json("GET", "/private");
      throw new Error("expected request to fail");
    } catch (error) {
      expect(error).toBeInstanceOf(TransportError);
      expect((error as TransportError).problem?.status).toBe(401);
      expect((error as TransportError).requestID).toBe("req-42");
    }
    expect(unauthorized).toBe(1);
  });

  test("handles non-JSON failures, empty responses, and malformed JSON", async () => {
    const failure = new HTTPTransport({
      baseURL: "https://example.test",
      fetch: (async () => new Response("offline", { status: 503, headers: { "content-type": "text/plain" } })) as typeof fetch,
    });
    await expect(failure.request("/status")).rejects.toMatchObject({ status: 503, problem: undefined });

    const empty = new HTTPTransport({
      baseURL: "https://example.test",
      fetch: (async () => new Response(null, { status: 204 })) as typeof fetch,
    });
    expect(await empty.request("/empty")).toBeUndefined();

    const malformed = new HTTPTransport({
      baseURL: "https://example.test",
      fetch: (async () => new Response("{", { status: 200, headers: { "content-type": "application/json" } })) as typeof fetch,
    });
    await expect(malformed.request("/broken")).rejects.toThrow("malformed JSON");
  });

  test("distinguishes timeout and caller abort", async () => {
    const waitingFetch = ((_input: RequestInfo | URL, init?: RequestInit) => new Promise<Response>((_resolve, reject) => {
      init?.signal?.addEventListener("abort", () => reject(new DOMException("aborted", "AbortError")), { once: true });
    })) as typeof fetch;
    const timed = new HTTPTransport({ baseURL: "https://example.test", timeoutMs: 5, fetch: waitingFetch });
    await expect(timed.request("/slow")).rejects.toThrow("timed out");

    const controller = new AbortController();
    const aborted = new HTTPTransport({ baseURL: "https://example.test", timeoutMs: 1000, fetch: waitingFetch });
    const request = aborted.request("/slow", { signal: controller.signal });
    controller.abort();
    await expect(request).rejects.toThrow("aborted");
  });
});
