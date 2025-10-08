import { t } from "@lingui/core/macro"
import { useStore } from "@nanostores/react"
import type { AuthMethodsList } from "pocketbase"
import { useEffect, useMemo, useState } from "react"
import { UserAuthForm } from "@/components/login/auth-form"
import { pb } from "@/lib/api"
import { Logo } from "../logo"
import { ModeToggle } from "../mode-toggle"
import { $router } from "../router"
import { useTheme } from "../theme-provider"
import ForgotPassword from "./forgot-pass-form"
import { OtpRequestForm } from "./otp-forms"

export default function () {
	const page = useStore($router)
	const [isFirstRun, setFirstRun] = useState(false)
	const [authMethods, setAuthMethods] = useState<AuthMethodsList>()
	const { theme } = useTheme()

	useEffect(() => {
		document.title = t`Login` + " / Beszel"

		pb.send("/api/beszel/first-run", {}).then(({ firstRun }) => {
			setFirstRun(firstRun)
		})
	}, [])

	useEffect(() => {
		pb.collection("users")
			.listAuthMethods()
			.then((methods) => {
				setAuthMethods(methods)
			})
	}, [])

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

	if (!authMethods) {
		return null
	}

	return (
		<div className="min-h-svh grid items-center py-12">
			<div
				className="grid gap-5 w-full px-4 mx-auto"
				// @ts-expect-error
				style={{ maxWidth: "21.5em", "--border": theme == "light" ? "hsl(30, 8%, 70%)" : "hsl(220, 3%, 25%)" }}
			>
				<div className="absolute top-3 right-3">
					<ModeToggle />
				</div>
				<div className="text-center">
					<h1 className="mb-3">
						<Logo className="h-7 fill-foreground mx-auto" />
						<span className="sr-only">Beszel</span>
					</h1>
					<p className="text-sm text-muted-foreground">{subtitle}</p>
				</div>
				{page?.route === "forgot_password" ? (
					<ForgotPassword />
				) : page?.route === "request_otp" ? (
					<OtpRequestForm />
				) : (
					<UserAuthForm isFirstRun={isFirstRun} authMethods={authMethods} />
				)}
			</div>
		</div>
	)
}
