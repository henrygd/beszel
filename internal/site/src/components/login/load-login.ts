import type PocketBase from "pocketbase"
import type { AuthMethodsList, ClientResponseError } from "pocketbase"

interface LoginContext {
	route?: string
	search: string
	loggedOut: boolean
	/** Server has AUTO_LOGIN or TRUSTED_AUTH_HEADER configured. */
	autoLogin: boolean
}

/** Last-resort auth methods so the login form still renders when the hub can't be reached. */
const fallbackAuthMethods: AuthMethodsList = {
	mfa: { enabled: false, duration: 0 },
	otp: { enabled: false, duration: 0 },
	password: { enabled: true, identityFields: ["email"] },
	oauth2: { enabled: false, providers: [] },
}

/**
 * Load auth methods with brief retries. A hub that is still starting up (e.g. right after
 * a host reboot) may fail this request while others already respond, and the login page
 * renders nothing at all if it never resolves, forcing a manual refresh. Falls back to the
 * password form: a rendered form beats a blank page, even if the real config differs.
 */
async function loadAuthMethods(pb: PocketBase, signal: AbortSignal): Promise<AuthMethodsList> {
	let lastError: unknown
	for (let attempt = 0; attempt < 3; attempt++) {
		if (attempt > 0) {
			await new Promise((resolve) => setTimeout(resolve, 250 * 2 ** (attempt - 1)))
			if (signal.aborted) throw lastError
		}
		try {
			return await pb.collection("users").listAuthMethods({ signal, requestKey: null })
		} catch (error) {
			lastError = error
			// aborted requests must propagate immediately: the page is gone or the probe timed out
			if ((error as ClientResponseError)?.isAbort || signal.aborted) throw error
		}
	}
	console.error("Failed to load auth methods, showing the password login form as fallback:", lastError)
	return fallbackAuthMethods
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
		loadAuthMethods(pb, signal),
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
