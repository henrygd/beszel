/**
 * Test setup, loaded by bun through bunfig.toml before any test file.
 *
 * Modules of the site read localStorage and sessionStorage at import time (the log timestamp
 * toggle in lib/stores.ts, useBrowserStorage in lib/utils.ts). Bun has no web storage, and a
 * module that throws while it is being evaluated stays broken in the registry, so a single
 * import of lib/stores took down every test file that touched it, whatever its own setup did.
 */
function memoryStorage(): Storage {
	const values = new Map<string, string>()
	return {
		get length() {
			return values.size
		},
		key: (index: number) => [...values.keys()][index] ?? null,
		getItem: (key: string) => values.get(key) ?? null,
		setItem: (key: string, value: string) => {
			values.set(key, String(value))
		},
		removeItem: (key: string) => {
			values.delete(key)
		},
		clear: () => values.clear(),
	}
}

for (const name of ["localStorage", "sessionStorage"] as const) {
	if (!(name in globalThis)) {
		Object.defineProperty(globalThis, name, { value: memoryStorage(), configurable: true, writable: true })
	}
}
