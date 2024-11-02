import { cn } from "@/lib/utils"
import { buttonVariants } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { LoaderCircle, LockIcon, LogInIcon, MailIcon, UserIcon } from "lucide-react"
import { $authenticated, pb } from "@/lib/stores"
import * as v from "valibot"
import { toast } from "../ui/use-toast"
import { Dialog, DialogContent, DialogTrigger, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import { useCallback, useState } from "react"
import { AuthMethodsList, OAuth2AuthConfig } from "pocketbase"
import { Link } from "../router"
import { Trans, t } from "@lingui/macro"

const honeypot = v.literal("")
const emailSchema = v.pipe(v.string(), v.email(t`Invalid email address.`))
const passwordSchema = v.pipe(v.string(), v.minLength(10, t`Password must be at least 10 characters.`))

const LoginSchema = v.looseObject({
	name: honeypot,
	email: emailSchema,
	password: passwordSchema,
})

const RegisterSchema = v.looseObject({
	name: honeypot,
	username: v.pipe(
		v.string(),
		v.regex(
			/^(?=.*[a-zA-Z])[a-zA-Z0-9_-]+$/,
			"Invalid username. You may use alphanumeric characters, underscores, and hyphens."
		),
		v.minLength(3, "Username must be at least 3 characters long.")
	),
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
				const { email, password, passwordConfirm, username } = result.output
				if (isFirstRun) {
					// check that passwords match
					if (password !== passwordConfirm) {
						let msg = "Passwords do not match"
						setErrors({ passwordConfirm: msg })
						return
					}
					await pb.admins.create({
						email,
						password,
						passwordConfirm: password,
					})
					await pb.admins.authWithPassword(email, password)
					await pb.collection("users").create({
						username,
						email,
						password,
						passwordConfirm: password,
						role: "admin",
						verified: true,
					})
					await pb.collection("users").authWithPassword(email, password)
				} else {
					await pb.collection("users").authWithPassword(email, password)
				}
				$authenticated.set(true)
			} catch (e) {
				showLoginFaliedToast()
			} finally {
				setIsLoading(false)
			}
		},
		[isFirstRun]
	)

	if (!authMethods) {
		return null
	}

	return (
		<div className={cn("grid gap-6", className)} {...props}>
			{authMethods.emailPassword && (
				<>
					<form onSubmit={handleSubmit} onChange={() => setErrors({})}>
						<div className="grid gap-2.5">
							{isFirstRun && (
								<div className="grid gap-1 relative">
									<UserIcon className="absolute left-3 top-3 h-4 w-4 text-muted-foreground" />
									<Label className="sr-only" htmlFor="username">
										<Trans>Username</Trans>
									</Label>
									<Input
										autoFocus={true}
										id="username"
										name="username"
										required
										placeholder={t`username`}
										type="username"
										autoCapitalize="none"
										autoComplete="username"
										autoCorrect="off"
										disabled={isLoading || isOauthLoading}
										className="ps-9"
									/>
									{errors?.username && <p className="px-1 text-xs text-red-600">{errors.username}</p>}
								</div>
							)}
							<div className="grid gap-1 relative">
								<MailIcon className="absolute left-3 top-3 h-4 w-4 text-muted-foreground" />
								<Label className="sr-only" htmlFor="email">
									<Trans>Email</Trans>
								</Label>
								<Input
									id="email"
									name="email"
									required
									placeholder={isFirstRun ? t`email` : "name@example.com"}
									type="email"
									autoCapitalize="none"
									autoComplete="email"
									autoCorrect="off"
									disabled={isLoading || isOauthLoading}
									className="ps-9"
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
									className="ps-9 lowercase"
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
										className="ps-9 lowercase"
									/>
									{errors?.passwordConfirm && <p className="px-1 text-xs text-red-600">{errors.passwordConfirm}</p>}
								</div>
							)}
							<div className="sr-only">
								{/* honeypot */}
								<label htmlFor="name"></label>
								<input id="name" type="text" name="name" tabIndex={-1} />
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
					{(isFirstRun || authMethods.authProviders.length > 0) && (
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

			{authMethods.authProviders.length > 0 && (
				<div className="grid gap-2 -mt-1">
					{authMethods.authProviders.map((provider) => (
						<button
							key={provider.name}
							type="button"
							className={cn(buttonVariants({ variant: "outline" }), {
								"justify-self-center": !authMethods.emailPassword,
								"px-5": !authMethods.emailPassword,
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
									className="me-2 h-4 w-4 dark:invert"
									src={`/static/${provider.name}.svg`}
									alt=""
									onError={(e) => {
										e.currentTarget.src = "/static/lock.svg"
									}}
								/>
							)}
							<span className="translate-y-[1px]">{provider.displayName}</span>
						</button>
					))}
				</div>
			)}

			{!authMethods.authProviders.length && isFirstRun && (
				// only show GitHub button / dialog during onboarding
				<Dialog>
					<DialogTrigger asChild>
						<button type="button" className={cn(buttonVariants({ variant: "outline" }))}>
							<img className="me-2 h-4 w-4 dark:invert" src="/static/github.svg" alt="" />
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
										href="https://github.com/henrygd/beszel/blob/main/readme.md#oauth--oidc-integration"
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

			{authMethods.emailPassword && !isFirstRun && (
				<Link
					href="/forgot-password"
					className="text-sm mx-auto hover:text-brand underline underline-offset-4 opacity-70 hover:opacity-100 transition-opacity"
				>
					<Trans>Forgot password?</Trans>
				</Link>
			)}
		</div>
	)
}
