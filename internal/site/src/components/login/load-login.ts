import type PocketBase from "pocketbase"

/** Load the login page only after automatic / trusted-header authentication has been tried. */
export async function loadLogin(
	pb: PocketBase,
	{ route, search, loggedOut }: { route?: string; search: string; loggedOut: boolean },
	signal: AbortSignal
) {
	const [{ firstRun }, authMethods] = await Promise.all([
		pb
			.send<{ firstRun: boolean }>("/api/beszel/first-run", { signal, requestKey: null })
			.catch(() => ({ firstRun: false })),
		pb.collection("users").listAuthMethods({ signal, requestKey: null }),
	])
	const result = { firstRun, authMethods, authenticated: false }
	// These flows are handled by their existing forms, without a session probe.
	if (
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
	const timeout = setTimeout(abort, 10_000)
	try {
		await pb.collection("users").authRefresh({ signal: controller.signal, requestKey: null })
		result.authenticated = pb.authStore.isValid
	} catch {
		// A rejected anonymous refresh is expected when automatic login is not configured.
	} finally {
		clearTimeout(timeout)
		signal.removeEventListener("abort", abort)
	}
	return result
}
