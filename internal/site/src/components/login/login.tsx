import { t } from "@lingui/core/macro"
import { Trans } from "@lingui/react/macro"
import { useStore } from "@nanostores/react"
import { ActivityIcon, LoaderCircleIcon, RadioTowerIcon, RefreshCwIcon, ShieldCheckIcon } from "lucide-react"
import type { AuthMethodsList } from "pocketbase"
import { useCallback, useEffect, useMemo, useState } from "react"
import { UserAuthForm } from "@/components/login/auth-form"
import { Button } from "@/components/ui/button"
import { Card, CardContent } from "@/components/ui/card"
import { pb } from "@/lib/api"
import { Logo } from "../logo"
import { ModeToggle } from "../mode-toggle"
import { $router } from "../router"
import ForgotPassword from "./forgot-pass-form"
import { OtpRequestForm } from "./otp-forms"

export default function () {
	const page = useStore($router)
	const [isFirstRun, setFirstRun] = useState(false)
	const [authMethods, setAuthMethods] = useState<AuthMethodsList>()
	const [loadError, setLoadError] = useState(false)

	const loadLogin = useCallback(async () => {
		setLoadError(false)
		try {
			const [{ firstRun }, methods] = await Promise.all([
				pb.send<{ firstRun: boolean }>("/api/beszel/first-run", {}),
				pb.collection("users").listAuthMethods(),
			])
			setFirstRun(firstRun)
			setAuthMethods(methods)
		} catch {
			setLoadError(true)
		}
	}, [])

	useEffect(() => {
		document.title = `${t`Login`} / Beszel`
		loadLogin()
	}, [loadLogin])

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

	return (
		<div className="grid min-h-svh lg:grid-cols-[minmax(24rem,0.9fr)_minmax(30rem,1.1fr)]">
			<aside className="relative hidden overflow-hidden border-r bg-[#071721] p-10 text-white lg:flex lg:flex-col lg:justify-between">
				<div className="signal-scan absolute inset-0 opacity-40" aria-hidden="true" />
				<div className="relative flex items-center gap-3">
					<div className="grid size-10 place-items-center rounded-xl border border-white/15 bg-white/5">
						<RadioTowerIcon className="size-5 text-[#31d6c3]" />
					</div>
					<Logo className="h-7 fill-white" />
				</div>
				<div className="relative max-w-xl">
					<p className="mb-4 font-mono text-xs font-semibold uppercase tracking-[0.25em] text-[#31d6c3]">
						<Trans>Self-hosted observability</Trans>
					</p>
					<h1 className="text-balance text-4xl font-semibold leading-tight tracking-tight xl:text-5xl">
						<Trans>Your fleet, clearly in view.</Trans>
					</h1>
					<p className="mt-5 max-w-lg text-pretty leading-7 text-white/65">
						<Trans>
							Monitor system health, resource pressure, services, containers, and alerts from one private control plane.
						</Trans>
					</p>
				</div>
				<div className="relative grid grid-cols-3 gap-3 text-xs text-white/65">
					<LoginCapability icon={ActivityIcon} label={<Trans>Live telemetry</Trans>} />
					<LoginCapability icon={ShieldCheckIcon} label={<Trans>Private by design</Trans>} />
					<LoginCapability icon={RadioTowerIcon} label={<Trans>Lightweight agents</Trans>} />
				</div>
			</aside>

			<main className="relative grid place-items-center px-4 py-12 sm:px-8">
				<div className="absolute right-4 top-4">
					<ModeToggle />
				</div>
				<div className="w-full max-w-md">
					<div className="mb-6 flex items-center justify-center gap-2 lg:hidden">
						<RadioTowerIcon className="size-5 text-primary" />
						<Logo className="h-7 fill-foreground" />
					</div>
					<Card className="login-card-glow">
						<CardContent className="p-6 sm:p-8">
							<div className="mb-7">
								<p className="font-mono text-[0.68rem] font-semibold uppercase tracking-[0.2em] text-primary">
									<Trans>Beszel control plane</Trans>
								</p>
								<h2 className="mt-2 text-2xl font-semibold tracking-tight">
									{isFirstRun ? <Trans>Create administrator</Trans> : <Trans>Welcome back</Trans>}
								</h2>
								<p className="mt-2 text-sm text-muted-foreground">{subtitle}</p>
							</div>
							{loadError ? (
								<div className="rounded-xl border border-destructive/30 bg-destructive/5 p-4 text-sm">
									<p className="font-medium">
										<Trans>Unable to load sign-in options.</Trans>
									</p>
									<Button variant="outline" className="mt-4" onClick={() => loadLogin()}>
										<RefreshCwIcon className="size-4" />
										<Trans>Try again</Trans>
									</Button>
								</div>
							) : !authMethods ? (
								<output className="grid min-h-40 place-items-center">
									<LoaderCircleIcon className="size-6 animate-spin text-primary" />
									<span className="sr-only">
										<Trans>Loading</Trans>
									</span>
								</output>
							) : page?.route === "forgot_password" ? (
								<ForgotPassword />
							) : page?.route === "request_otp" ? (
								<OtpRequestForm />
							) : (
								<UserAuthForm isFirstRun={isFirstRun} authMethods={authMethods} />
							)}
						</CardContent>
					</Card>
					<p className="mt-5 text-center font-mono text-[0.68rem] uppercase tracking-[0.15em] text-muted-foreground">
						<Trans>Secure access to your infrastructure</Trans>
					</p>
				</div>
			</main>
		</div>
	)
}

function LoginCapability({ icon: Icon, label }: { icon: typeof ActivityIcon; label: React.ReactNode }) {
	return (
		<div className="flex items-center gap-2 rounded-lg border border-white/10 bg-white/[0.035] px-3 py-2.5">
			<Icon className="size-3.5 shrink-0 text-[#31d6c3]" aria-hidden="true" />
			<span>{label}</span>
		</div>
	)
}
