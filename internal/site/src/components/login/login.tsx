import { t } from "@lingui/core/macro"
import { Trans } from "@lingui/react/macro"
import { useStore } from "@nanostores/react"
import { ActivityIcon, LoaderCircleIcon, RefreshCwIcon, ShieldCheckIcon, ZapIcon } from "lucide-react"
import type { AuthMethodsList } from "pocketbase"
import { useCallback, useEffect, useMemo, useState } from "react"
import { UserAuthForm } from "@/components/login/auth-form"
import { Button } from "@/components/ui/button"
import { pb } from "@/lib/api"
import { Logo } from "../logo"
import { ModeToggle } from "../mode-toggle"
import { $router } from "../router"
import ForgotPassword from "./forgot-pass-form"
import { OtpRequestForm } from "./otp-forms"

/** Stylized rack of machines, each with a breathing status LED. */
const RACK_ROWS = [
	{ name: "web-01", status: "up", load: 34 },
	{ name: "web-02", status: "up", load: 21 },
	{ name: "lb-edge", status: "up", load: 12 },
	{ name: "db-primary", status: "up", load: 67 },
	{ name: "db-replica", status: "up", load: 44 },
	{ name: "cache-01", status: "up", load: 18 },
	{ name: "media", status: "down", load: 0 },
	{ name: "backup-01", status: "up", load: 26 },
]

function RackPreview() {
	return (
		<div
			aria-hidden="true"
			className="w-full max-w-sm rounded-xl border border-white/10 bg-white/[0.03] p-2 shadow-2xl shadow-black/40 backdrop-blur-sm"
		>
			{RACK_ROWS.map((row, i) => (
				<div
					key={row.name}
					className="flex items-center gap-3 rounded-lg px-3 py-2.5 [&:not(:last-child)]:border-b [&:not(:last-child)]:border-white/5"
				>
					<span className={`led led-${row.status} led-live size-1.5`} style={{ animationDelay: `${i * 0.4}s` }} />
					<span className="w-24 shrink-0 font-mono text-xs text-white/55">{row.name}</span>
					<span className="h-1 flex-1 overflow-hidden rounded-full bg-white/8">
						<span
							className="block h-full rounded-full bg-gradient-to-r from-[hsl(170_60%_45%/0.7)] to-[hsl(170_60%_55%/0.9)]"
							style={{ width: row.status === "down" ? "4%" : `${row.load}%` }}
						/>
					</span>
					<span className="w-9 shrink-0 text-end font-mono text-[0.65rem] tabular-nums text-white/40">
						{row.status === "down" ? "—" : `${row.load}%`}
					</span>
				</div>
			))}
		</div>
	)
}

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
		<div className="grid min-h-svh lg:grid-cols-[1.1fr_1fr]">
			{/* Presentation panel — the fleet as a rack of live machines */}
			<aside className="bg-blueprint relative hidden flex-col justify-between overflow-hidden bg-[hsl(222_30%_7%)] p-10 text-white lg:flex xl:p-14">
				<div
					aria-hidden="true"
					className="absolute inset-0 bg-[radial-gradient(ellipse_70%_50%_at_20%_0%,hsl(172_60%_20%/0.5),transparent),radial-gradient(ellipse_50%_40%_at_90%_100%,hsl(215_70%_20%/0.4),transparent)]"
				/>
				<div className="relative flex items-center gap-3">
					<span className="grid size-9 place-items-center rounded-lg border border-white/15 bg-white/5">
						<ZapIcon className="size-4 text-[hsl(170_60%_50%)]" />
					</span>
					<Logo className="h-6 fill-white" />
				</div>
				<div className="relative grid gap-10">
					<div className="max-w-lg">
						<p className="font-mono text-[0.65rem] font-semibold uppercase tracking-[0.2em] text-[hsl(170_55%_52%)]">
							<Trans>Self-hosted observability</Trans>
						</p>
						<h1 className="mt-4 text-balance text-4xl font-semibold leading-tight xl:text-[2.75rem]">
							<Trans>Your fleet, clearly in view.</Trans>
						</h1>
						<p className="mt-5 max-w-md text-pretty leading-7 text-white/60">
							<Trans>
								Monitor system health, resource pressure, services, containers, and alerts from one private control
								plane.
							</Trans>
						</p>
					</div>
					<RackPreview />
				</div>
				<div className="relative flex flex-wrap gap-x-6 gap-y-2 text-xs text-white/50">
					<LoginCapability icon={ActivityIcon} label={<Trans>Live telemetry</Trans>} />
					<LoginCapability icon={ShieldCheckIcon} label={<Trans>Private by design</Trans>} />
					<LoginCapability icon={ZapIcon} label={<Trans>Lightweight agents</Trans>} />
				</div>
			</aside>

			{/* Form panel */}
			<main className="relative grid place-items-center px-4 py-12 sm:px-8">
				<div className="absolute end-4 top-4">
					<ModeToggle />
				</div>
				<div className="w-full max-w-sm">
					<div className="mb-8 flex items-center justify-center gap-2 lg:hidden">
						<Logo className="h-6 fill-foreground" />
					</div>
					<div className="mb-7">
						<h2 className="font-display text-[1.65rem] font-semibold leading-tight tracking-tight">
							{isFirstRun ? <Trans>Create administrator</Trans> : <Trans>Welcome back</Trans>}
						</h2>
						<p className="mt-1.5 text-sm text-muted-foreground">{subtitle}</p>
					</div>
					{loadError ? (
						<div className="rounded-lg border border-destructive/30 bg-destructive/5 p-4 text-sm">
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
					<p className="mt-8 text-center font-mono text-[0.65rem] uppercase tracking-[0.16em] text-muted-foreground/70">
						<Trans>Secure access to your infrastructure</Trans>
					</p>
				</div>
			</main>
		</div>
	)
}

function LoginCapability({ icon: Icon, label }: { icon: typeof ActivityIcon; label: React.ReactNode }) {
	return (
		<div className="flex items-center gap-2">
			<Icon className="size-3.5 shrink-0 text-[hsl(170_55%_50%)]" aria-hidden="true" />
			<span>{label}</span>
		</div>
	)
}
