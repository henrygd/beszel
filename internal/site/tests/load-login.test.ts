import { describe, expect, test } from "bun:test"
import PocketBase, { type AuthMethodsList } from "pocketbase"
import { loadLogin } from "../src/components/login/load-login"

const context = { route: "home", search: "", loggedOut: false, autoLogin: true }
const methods: AuthMethodsList = {
	password: { enabled: true, identityFields: ["email"] },
	oauth2: { enabled: false, providers: [] },
	otp: { enabled: false, duration: 180 },
	mfa: { enabled: false, duration: 180 },
}
const token = `test.${btoa(JSON.stringify({ exp: Math.floor(Date.now() / 1000) + 3600 }))}.test`
const success = () => Response.json({ token, record: { id: "test-user", collectionName: "users" } })

function client({
	firstRun = false,
	firstRunFailed = false,
	authMethods = methods,
	refresh = (_signal: AbortSignal) => Promise.resolve(success()),
} = {}) {
	const pb = new PocketBase("http://localhost/beszel/")
	const requests: string[] = []
	pb.beforeSend = (url, options) => ({
		url,
		options: {
			...options,
			fetch: () => {
				const path = new URL(url).pathname
				requests.push(path)
				if (path.endsWith("/first-run")) {
					return Promise.resolve(Response.json({ firstRun }, { status: firstRunFailed ? 500 : 200 }))
				}
				if (path.endsWith("/auth-methods")) return Promise.resolve(Response.json(authMethods))
				if (path.endsWith("/auth-refresh")) return refresh(options.signal as AbortSignal)
				throw new Error(`Unexpected request: ${path}`)
			},
		},
	})
	return { pb, requests }
}

describe("initial login authentication", () => {
	test("waits for automatic authentication and saves the session before resolving", async () => {
		const response = Promise.withResolvers<Response>()
		const started = Promise.withResolvers<void>()
		const { pb, requests } = client({
			refresh: () => {
				started.resolve()
				return response.promise
			},
		})
		let resolved = false
		const loading = loadLogin(pb, context, new AbortController().signal).then((result) => {
			resolved = true
			return result
		})
		await started.promise
		expect(resolved).toBe(false)
		expect(pb.authStore.isValid).toBe(false)
		response.resolve(success())
		expect((await loading).authenticated).toBe(true)
		expect(pb.authStore.record?.id).toBe("test-user")
		expect(pb.authStore.isValid).toBe(true)
		expect(requests.filter((path) => path.endsWith("/auth-refresh"))).toEqual([
			"/beszel/api/collections/users/auth-refresh",
		])
	})

	for (const failure of ["rejected", "network"] as const) {
		test(`${failure} authentication makes the password form available`, async () => {
			const { pb } = client({
				refresh: () => {
					if (failure === "network") return Promise.reject(new TypeError("Failed to fetch"))
					return Promise.resolve(Response.json({ message: "Unauthorized" }, { status: 401 }))
				},
			})
			const result = await loadLogin(pb, context, new AbortController().signal)
			expect(result.authenticated).toBe(false)
			expect(result.authMethods.password.enabled).toBe(true)
		})
	}

	test("does not probe when automatic authentication is not configured", async () => {
		const { pb, requests } = client()
		const result = await loadLogin(pb, { ...context, autoLogin: false }, new AbortController().signal)
		expect(result.authenticated).toBe(false)
		expect(result.authMethods.password.enabled).toBe(true)
		expect(requests.some((path) => path.endsWith("/auth-refresh"))).toBe(false)
	})

	for (const route of ["forgot_password", "request_otp"]) {
		test(`${route} does not attempt automatic authentication`, async () => {
			const { pb, requests } = client()
			await loadLogin(pb, { ...context, route }, new AbortController().signal)
			expect(requests.some((path) => path.endsWith("/auth-refresh"))).toBe(false)
		})
	}

	test("onboarding does not probe a nonexistent account", async () => {
		const { pb, requests } = client({ firstRun: true })
		const result = await loadLogin(pb, context, new AbortController().signal)
		expect(result.firstRun).toBe(true)
		expect(result.authenticated).toBe(false)
		expect(requests).toHaveLength(2)
	})

	test("an unavailable first-run check does not block authentication", async () => {
		const { pb } = client({ firstRunFailed: true })
		const result = await loadLogin(pb, context, new AbortController().signal)
		expect(result.firstRun).toBe(false)
		expect(result.authenticated).toBe(true)
	})

	test("OAuth callbacks retain priority over automatic authentication", async () => {
		const { pb, requests } = client()
		await loadLogin(pb, { ...context, search: "?code=example&state=test" }, new AbortController().signal)
		expect(requests).toHaveLength(2)
	})

	for (const loggedOut of [false, true]) {
		test(`single-provider OAuth preserves the logout guard (${loggedOut})`, async () => {
			const { pb, requests } = client({
				authMethods: {
					...methods,
					password: { ...methods.password, enabled: false },
					oauth2: { enabled: true, providers: [{ name: "oidc" } as AuthMethodsList["oauth2"]["providers"][number]] },
				},
			})
			await loadLogin(pb, { ...context, loggedOut }, new AbortController().signal)
			expect(requests.some((path) => path.endsWith("/auth-refresh"))).toBe(loggedOut)
		})
	}

	for (const cancel of [true, false]) {
		test(
			cancel ? "unmount cancels a pending probe" : "a stalled probe times out and releases the login form",
			async () => {
				const controller = new AbortController()
				const started = Promise.withResolvers<void>()
				let aborted = false
				const { pb } = client({
					refresh: (signal) =>
						new Promise<Response>((_resolve, reject) => {
							signal.addEventListener("abort", () => {
								aborted = true
								reject(new DOMException("Aborted", "AbortError"))
							})
							started.resolve()
						}),
				})
				const result = loadLogin(pb, context, controller.signal, cancel ? 10_000 : 20)
				await started.promise
				if (cancel) controller.abort()
				expect((await result).authenticated).toBe(false)
				expect(aborted).toBe(true)
			}
		)
	}
})
