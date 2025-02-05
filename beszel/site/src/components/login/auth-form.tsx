import { cn } from "@/lib/utils"
import { buttonVariants } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { LoaderCircle, LockIcon, LogInIcon, MailIcon } from "lucide-react"
import { $authenticated, pb } from "@/lib/stores"
import * as v from "valibot"
import { toast } from "../ui/use-toast"
import { Dialog, DialogContent, DialogTrigger, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import { useCallback, useState } from "react"
import { AuthMethodsList, OAuth2AuthConfig } from "pocketbase"
import { $router, Link, prependBasePath } from "../router"
import { Trans, t } from "@lingui/macro"
import { getPagePath } from "@nanostores/router"

const honeypot = v.literal("")
const emailSchema = v.pipe(v.string(), v.email(t`Invalid email address.`))
const passwordSchema = v.pipe(
	v.string(),
	v.minLength(8, t`Password must be at least 8 characters.`),
	v.maxBytes(72, t`Password must be less than 72 bytes.`)
)

const LoginSchema = v.looseObject({
	name: honeypot,
	email: emailSchema,
	password: passwordSchema,
})

const RegisterSchema = v.looseObject({
	name: honeypot,
	email: emailSchema,
	password: passwordSchema,
	passwordConfirm: passwordSchema,
})

const showLoginFaliedToast = () => {
	toast({
		title: t`Login attempt failed`,
		description: t`Please check your credentials and try again`,
		variant: "destructive",
	})
}

export function UserAuthForm({
	className,
	isFirstRun,
	authMethods,
	...props
}: {
	className?: string
	isFirstRun: boolean
	authMethods: AuthMethodsList
}) {
	const [isLoading, setIsLoading] = useState<boolean>(false)
	const [isOauthLoading, setIsOauthLoading] = useState<boolean>(false)
	const [errors, setErrors] = useState<Record<string, string | undefined>>({})

	const handleSubmit = useCallback(
		async (e: React.FormEvent<HTMLFormElement>) => {
			e.preventDefault()
			setIsLoading(true)
			// store email for later use if mfa is enabled
			let email = ""
			try {
				const formData = new FormData(e.target as HTMLFormElement)
				const data = Object.fromEntries(formData) as Record<string, any>
				const Schema = isFirstRun ? RegisterSchema : LoginSchema
				const result = v.safeParse(Schema, data)
				if (!result.success) {
					console.log(result)
					let errors = {}
					for (const issue of result.issues) {
						// @ts-ignore
						errors[issue.path[0].key] = issue.message
					}
					setErrors(errors)
					return
				}
				const { password, passwordConfirm } = result.output
				email = result.output.email
				if (isFirstRun) {
					// check that passwords match
					if (password !== passwordConfirm) {
						let msg = "Passwords do not match"
						setErrors({ passwordConfirm: msg })
						return
					}
					await pb.send("/api/beszel/create-user", {
						method: "POST",
						body: JSON.stringify({ email, password }),
					})
					await pb.collection("users").authWithPassword(email, password)
				} else {
					await pb.collection("users").authWithPassword(email, password)
				}
				$authenticated.set(true)
			} catch (err: any) {
				showLoginFaliedToast()
				// todo: implement MFA
				// const mfaId = err.response?.mfaId
				// if (!mfaId) {
				// 	showLoginFaliedToast()
				// 	throw err
				// }
				// the user needs to authenticate again with another auth method, for example OTP
				// const result = await pb.collection("users").requestOTP(email)
				// ... show a modal for users to check their email and to enter the received code ...
				// await pb.collection("users").authWithOTP(result.otpId, "EMAIL_CODE", { mfaId: mfaId })
			} finally {
				setIsLoading(false)
			}
		},
		[isFirstRun]
	)

	if (!authMethods) {
		return null
	}

	const oauthEnabled = authMethods.oauth2.enabled && authMethods.oauth2.providers.length > 0
	const passwordEnabled = authMethods.password.enabled

	return (
		<div className={cn("grid gap-6", className)} {...props}>
			{passwordEnabled && (
				<>
					<form onSubmit={handleSubmit} onChange={() => setErrors({})}>
						<div className="grid gap-2.5">
							<div className="grid gap-1 relative">
								<MailIcon className="absolute left-3 top-3 h-4 w-4 text-muted-foreground" />
								<Label className="sr-only" htmlFor="email">
									<Trans>Email</Trans>
								</Label>
								<Input
									id="email"
									name="email"
									required
									placeholder="name@example.com"
									type="text"
									autoCapitalize="none"
									autoComplete="email"
									autoCorrect="off"
									disabled={isLoading || isOauthLoading}
									className={cn("ps-9", errors?.email && "border-red-500")}
								/>
								{errors?.email && <p className="px-1 text-xs text-red-600">{errors.email}</p>}
							</div>
							<div className="grid gap-1 relative">
								<LockIcon className="absolute left-3 top-3 h-4 w-4 text-muted-foreground" />
								<Label className="sr-only" htmlFor="pass">
									<Trans>Password</Trans>
								</Label>
								<Input
									id="pass"
									name="password"
									placeholder={t`Password`}
									required
									type="password"
									autoComplete="current-password"
									disabled={isLoading || isOauthLoading}
									className={cn("ps-9", errors?.password && "border-red-500")}
								/>
								{errors?.password && <p className="px-1 text-xs text-red-600">{errors.password}</p>}
							</div>
							{isFirstRun && (
								<div className="grid gap-1 relative">
									<LockIcon className="absolute left-3 top-3 h-4 w-4 text-muted-foreground" />
									<Label className="sr-only" htmlFor="pass2">
										<Trans>Confirm password</Trans>
									</Label>
									<Input
										id="pass2"
										name="passwordConfirm"
										placeholder={t`Confirm password`}
										required
										type="password"
										autoComplete="current-password"
										disabled={isLoading || isOauthLoading}
										className={cn("ps-9", errors?.password && "border-red-500")}
									/>
									{errors?.passwordConfirm && <p className="px-1 text-xs text-red-600">{errors.passwordConfirm}</p>}
								</div>
							)}
							<div className="sr-only">
								{/* honeypot */}
								<label htmlFor="name"></label>
								<input id="name" type="text" name="name" tabIndex={-1} autoComplete="off" />
							</div>
							<button className={cn(buttonVariants())} disabled={isLoading}>
								{isLoading ? (
									<LoaderCircle className="me-2 h-4 w-4 animate-spin" />
								) : (
									<LogInIcon className="me-2 h-4 w-4" />
								)}
								{isFirstRun ? t`Create account` : t`Sign in`}
							</button>
						</div>
					</form>
					{(isFirstRun || oauthEnabled) && (
						// only show 'continue with' during onboarding or if we have auth providers
						<div className="relative">
							<div className="absolute inset-0 flex items-center">
								<span className="w-full border-t" />
							</div>
							<div className="relative flex justify-center text-xs uppercase">
								<span className="bg-background px-2 text-muted-foreground">
									<Trans>Or continue with</Trans>
								</span>
							</div>
						</div>
					)}
				</>
			)}

			{oauthEnabled && (
				<div className="grid gap-2 -mt-1">
					{authMethods.oauth2.providers.map((provider) => (
						<button
							key={provider.name}
							type="button"
							className={cn(buttonVariants({ variant: "outline" }), {
								"justify-self-center": !passwordEnabled,
								"px-5": !passwordEnabled,
							})}
							onClick={() => {
								setIsOauthLoading(true)
								const oAuthOpts: OAuth2AuthConfig = {
									provider: provider.name,
								}
								// https://github.com/pocketbase/pocketbase/discussions/2429#discussioncomment-5943061
								if (navigator.userAgent.match(/iPhone|iPad|iPod/i)) {
									const authWindow = window.open()
									if (!authWindow) {
										setIsOauthLoading(false)
										toast({
											title: t`Error`,
											description: t`Please enable pop-ups for this site`,
											variant: "destructive",
										})
										return
									}
									oAuthOpts.urlCallback = (url) => {
										authWindow.location.href = url
									}
								}
								pb.collection("users")
									.authWithOAuth2(oAuthOpts)
									.then(() => {
										$authenticated.set(pb.authStore.isValid)
									})
									.catch(showLoginFaliedToast)
									.finally(() => {
										setIsOauthLoading(false)
									})
							}}
							disabled={isLoading || isOauthLoading}
						>
							{isOauthLoading ? (
								<LoaderCircle className="me-2 h-4 w-4 animate-spin" />
							) : (
								<img
									className="me-2 h-4 w-4 dark:brightness-0 dark:invert"
									src={prependBasePath(`/_/images/oauth2/${provider.name}.svg`)}
									alt=""
									// onError={(e) => {
									// 	e.currentTarget.src = "/static/lock.svg"
									// }}
								/>
							)}
							<span className="translate-y-[1px]">{provider.displayName}</span>
						</button>
					))}
				</div>
			)}

			{!oauthEnabled && isFirstRun && (
				// only show GitHub button / dialog during onboarding
				<Dialog>
					<DialogTrigger asChild>
						<button type="button" className={cn(buttonVariants({ variant: "outline" }))}>
							<img className="me-2 h-4 w-4 dark:invert" src={prependBasePath("/_/images/oauth2/github.svg")} alt="" />
							<span className="translate-y-[1px]">GitHub</span>
						</button>
					</DialogTrigger>
					<DialogContent style={{ maxWidth: 440, width: "90%" }}>
						<DialogHeader>
							<DialogTitle>
								<Trans>OAuth 2 / OIDC support</Trans>
							</DialogTitle>
						</DialogHeader>
						<div className="text-primary/70 text-[0.95em] contents">
							<p>
								<Trans>Beszel supports OpenID Connect and many OAuth2 authentication providers.</Trans>
							</p>
							<p>
								<Trans>
									Please see{" "}
									<a
										href="https://beszel.dev/guide/oauth"
										className={cn(buttonVariants({ variant: "link" }), "p-0 h-auto")}
									>
										the documentation
									</a>{" "}
									for instructions.
								</Trans>
							</p>
						</div>
					</DialogContent>
				</Dialog>
			)}

			{passwordEnabled && !isFirstRun && (
				<Link
					href={getPagePath($router, "forgot_password")}
					className="text-sm mx-auto hover:text-brand underline underline-offset-4 opacity-70 hover:opacity-100 transition-opacity"
				>
					<Trans>Forgot password?</Trans>
				</Link>
			)}
		</div>
	)
}
