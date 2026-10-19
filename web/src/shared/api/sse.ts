import { apiUrl } from "./http";

/**
 * Server-Sent Events with automatic fallback to polling (SPEC-04 §3: poll every 3s if SSE fails).
 * Usage (infrastructure layer only):
 *   const stream = openEventStream(`/jobs/${id}/events`, {
 *     events: { status: onStatus, step: onStep, done: (d) => { onDone(d); stream.close(); } },
 *     fallbackPoll: () => refetchJob(),
 *   });
 */
export interface EventStreamOptions {
  /** Handlers keyed by SSE `event:` name; `data` is JSON-parsed (raw string if not JSON). */
  events: Record<string, (data: unknown) => void>;
  /** Called on each tick once SSE is unavailable/failed. */
  fallbackPoll?: () => unknown;
  pollIntervalMs?: number;
  onFallback?: () => void;
}

export interface EventStream {
  close: () => void;
  readonly mode: "sse" | "polling" | "closed";
}

function parse(raw: string): unknown {
  try {
    return JSON.parse(raw);
  } catch {
    return raw;
  }
}

export function openEventStream(path: string, opts: EventStreamOptions): EventStream {
  let mode: EventStream["mode"] = "sse";
  let source: EventSource | null = null;
  let timer: ReturnType<typeof setInterval> | null = null;

  const startPolling = () => {
    source?.close();
    source = null;
    if (mode === "closed") return;
    mode = "polling";
    opts.onFallback?.();
    if (!opts.fallbackPoll) return;
    void opts.fallbackPoll();
    timer = setInterval(() => void opts.fallbackPoll?.(), opts.pollIntervalMs ?? 3000);
  };

  if (typeof EventSource === "undefined") {
    startPolling();
  } else {
    source = new EventSource(apiUrl(path), { withCredentials: true });
    for (const [name, handler] of Object.entries(opts.events)) {
      source.addEventListener(name, (ev) => handler(parse((ev as MessageEvent<string>).data)));
    }
    source.onerror = startPolling;
  }

  return {
    close: () => {
      mode = "closed";
      source?.close();
      source = null;
      if (timer) clearInterval(timer);
      timer = null;
    },
    get mode() {
      return mode;
    },
  };
}
