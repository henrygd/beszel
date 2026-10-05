// Browser-global shims for bun:test: some modules (e.g. src/lib/stores.ts)
// read localStorage at import time, and bun's test runtime has no DOM.
if (typeof globalThis.localStorage === "undefined") {
	const store = new Map<string, string>()
	const shim = {
		getItem: (key: string) => (store.has(key) ? (store.get(key) as string) : null),
		setItem: (key: string, value: string) => store.set(key, String(value)),
		removeItem: (key: string) => store.delete(key),
		clear: () => store.clear(),
		key: (index: number) => [...store.keys()][index] ?? null,
		get length() {
			return store.size
		},
	}
	globalThis.localStorage = shim as Storage
	globalThis.sessionStorage = shim as Storage
}
