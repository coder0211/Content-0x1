/** Runs before any test import (MSW captures globals like Request at import time). */
// Node ≥ 22 exposes an experimental global `localStorage` that shadows jsdom's and is unusable
// without --localstorage-file; install an in-memory Storage instead.
class MemoryStorage implements Storage {
  #m = new Map<string, string>();
  get length() {
    return this.#m.size;
  }
  clear() {
    this.#m.clear();
  }
  getItem(k: string) {
    return this.#m.get(k) ?? null;
  }
  key(i: number) {
    return [...this.#m.keys()][i] ?? null;
  }
  removeItem(k: string) {
    this.#m.delete(k);
  }
  setItem(k: string, v: string) {
    this.#m.set(k, String(v));
  }
}
for (const name of ["localStorage", "sessionStorage"] as const) {
  if (typeof globalThis[name]?.clear !== "function") {
    Object.defineProperty(globalThis, name, { value: new MemoryStorage(), configurable: true });
  }
}

// jsdom's AbortSignal is not accepted by Node's (undici) Request, which React Router uses for
// navigations. Drop foreign signals in tests; aborting is not exercised by unit tests.
const NativeRequest = globalThis.Request;
class TestRequest extends NativeRequest {
  constructor(input: RequestInfo | URL, init?: RequestInit) {
    let ok = true;
    try {
      new NativeRequest(input, init);
    } catch {
      ok = false;
    }
    if (ok || !init?.signal) {
      super(input, init);
    } else {
      const { signal: _signal, ...rest } = init;
      super(input, rest);
    }
  }
}
globalThis.Request = TestRequest as typeof Request;

// Radix/Floating UI rely on these browser APIs that jsdom lacks.
class ResizeObserverStub {
  observe() {}
  unobserve() {}
  disconnect() {}
}
globalThis.ResizeObserver ??= ResizeObserverStub as unknown as typeof ResizeObserver;
Element.prototype.scrollIntoView ??= () => {};
Element.prototype.hasPointerCapture ??= () => false;
Element.prototype.releasePointerCapture ??= () => {};
window.matchMedia ??= ((query: string) => ({
  matches: false,
  media: query,
  onchange: null,
  addListener: () => {},
  removeListener: () => {},
  addEventListener: () => {},
  removeEventListener: () => {},
  dispatchEvent: () => false,
})) as typeof window.matchMedia;

export {};
