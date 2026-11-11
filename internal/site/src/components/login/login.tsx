import { t } from "@lingui/core/macro"
import { useStore } from "@nanostores/react"
import { useEffect, useMemo, useState } from "react"
import { UserAuthForm } from "@/components/login/auth-form"
import { pb } from "@/lib/api"
import { $authenticated } from "@/lib/stores"
import { Logo } from "../logo"
import { ModeToggle } from "../mode-toggle"
import { $router } from "../router"
import { useTheme } from "../theme-provider"
import ForgotPassword from "./forgot-pass-form"
import { loadLogin } from "./load-login"
import { OtpRequestForm } from "./otp-forms"

export default function () {
	const page = useStore($router)
	const [login, setLogin] = useState<Awaited<ReturnType<typeof loadLogin>> & { route?: string }>()
	const isFirstRun = login?.firstRun ?? false
	const { resolvedTheme } = useTheme()

	useEffect(() => {
		const controller = new AbortController()
		document.title = "Beszel"
		loadLogin(
			pb,
			{ route: page?.route, search: window.location.search, loggedOut: !!sessionStorage.getItem("lo") },
			controller.signal
		)
			.then(({ firstRun, authMethods, authenticated }) => {
				if (controller.signal.aborted) return
				if (authenticated) {
					$authenticated.set(true)
					return
				}
				document.title = `${t`Login`} / Beszel`
				setLogin({ firstRun, authMethods, authenticated, route: page?.route })
			})
			.catch((error) => {
				if (!controller.signal.aborted) console.error(error)
			})
		return () => controller.abort()
	}, [page?.route])

	const subtitle = useMemo(() => {
		if (isFirstRun) {
			return t`Please create an admin account`
		} else if (page?.route === "forgot_password") {
			return t`Enter email address to reset password`
		} else if (page?.route === "request_otp") {
			return t`Request a one-time password`
		} else {
			return t`Please sign in to your account`
		}
	}, [isFirstRun, page])

	if (!login || login.route !== page?.route) {
		return null
	}

	return (
		<div className="min-h-svh grid items-center py-12">
			<div
				className="grid gap-5 w-full px-4 mx-auto"
				// @ts-expect-error
				style={{ maxWidth: "21.5em", "--border": resolvedTheme === "light" ? "hsl(30, 8%, 70%)" : "hsl(220, 3%, 25%)" }}
			>
				<div className="absolute top-3 right-3">
					<ModeToggle />
				</div>
				<div className="text-center">
					<h1 className="mb-3">
						{/* Logo's aria-label provides the heading text */}
						<Logo className="h-7 fill-foreground mx-auto" />
					</h1>
					<p className="text-sm text-muted-foreground">{subtitle}</p>
				</div>
				{page?.route === "forgot_password" ? (
					<ForgotPassword />
				) : page?.route === "request_otp" ? (
					<OtpRequestForm />
				) : (
					<UserAuthForm isFirstRun={isFirstRun} authMethods={login.authMethods} />
				)}
			</div>
		</div>
	)
}
