// lib/stores reads localStorage on import, which bun does not provide. Without it the
// first test file to load stores fails, and every later one with it.
globalThis.localStorage ??= { getItem: () => null, setItem: () => {}, removeItem: () => {} } as unknown as Storage
