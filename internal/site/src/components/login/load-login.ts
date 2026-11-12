import type PocketBase from "pocketbase"

interface LoginContext {
	route?: string
	search: string
	loggedOut: boolean
	/** Server has AUTO_LOGIN or TRUSTED_AUTH_HEADER configured. */
	autoLogin: boolean
}

/**
 * Load login page data. When automatic / trusted-header authentication is configured,
 * try it before resolving so the form isn't shown to users who will be logged in anyway.
 */
export async function loadLogin(
	pb: PocketBase,
	{ route, search, loggedOut, autoLogin }: LoginContext,
	signal: AbortSignal,
	timeoutMs = 10_000
) {
	const [{ firstRun }, authMethods] = await Promise.all([
		pb
			.send<{ firstRun: boolean }>("/api/beszel/first-run", { signal, requestKey: null })
			.catch(() => ({ firstRun: false })),
		pb.collection("users").listAuthMethods({ signal, requestKey: null }),
	])
	const result = { firstRun, authMethods, authenticated: false }
	// Only probe when the server can authenticate an anonymous request. Other flows are
	// handled by their existing forms.
	if (
		!autoLogin ||
		signal.aborted ||
		firstRun ||
		route === "forgot_password" ||
		route === "request_otp" ||
		new URLSearchParams(search).get("code") ||
		(!authMethods.password.enabled && authMethods.oauth2.providers.length === 1 && !loggedOut)
	) {
		return result
	}

	const controller = new AbortController()
	const abort = () => controller.abort()
	signal.addEventListener("abort", abort, { once: true })
	// A stalled probe must not prevent access to the regular login form.
	const timeout = setTimeout(abort, timeoutMs)
	try {
		await pb.collection("users").authRefresh({ signal: controller.signal, requestKey: null })
		result.authenticated = pb.authStore.isValid
	} catch {
		// Rejected or timed out: fall back to the login form.
	} finally {
		clearTimeout(timeout)
		signal.removeEventListener("abort", abort)
	}
	return result
}
