import { expect, test } from "bun:test"
import PocketBase from "pocketbase"
import { loadLogin } from "../src/components/login/load-login"

test.each([
	["first-run", false],
	["auth-methods", false],
	["both", false],
	["first-run", true],
	["auth-methods", true],
	["both", true],
] as const)("a stalled %s request releases the login form (auto login %p)", async (stalledPath, autoLogin) => {
	const pb = new PocketBase("http://localhost")
	let aborted = false
	const requests: string[] = []
	pb.beforeSend = (url, options) => ({
		url,
		options: {
			...options,
			fetch: () => {
				requests.push(url)
				if (stalledPath === "both" || url.endsWith(stalledPath)) {
					return new Promise<Response>((_resolve, reject) => {
						options.signal?.addEventListener("abort", () => {
							aborted = true
							reject(new DOMException("Aborted", "AbortError"))
						})
					})
				}
				return Promise.resolve(
					Response.json(
						url.endsWith("first-run")
							? { firstRun: false }
							: {
									password: { enabled: true, identityFields: ["email"] },
									oauth2: { enabled: false, providers: [] },
									otp: { enabled: false, duration: 0 },
									mfa: { enabled: false, duration: 0 },
								}
					)
				)
			},
		},
	})
	const loading = loadLogin(
		pb,
		{ route: "home", search: "", loggedOut: false, autoLogin },
		new AbortController().signal,
		20
	)
	const outcome = await Promise.race([
		loading.then((result) => ({ result })),
		new Promise<"still pending">((resolve) => setTimeout(() => resolve("still pending"), 250)),
	])
	expect(outcome).not.toBe("still pending")
	expect(aborted).toBe(true)
	expect(requests.some((url) => url.endsWith("auth-refresh"))).toBe(false)
	if (outcome !== "still pending") {
		expect(outcome.result.authenticated).toBe(false)
		expect(outcome.result.authMethods.password.enabled).toBe(true)
	}
})
