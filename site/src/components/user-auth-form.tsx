import * as React from 'react'
import { cn } from '@/lib/utils'
import { buttonVariants } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Github, LoaderCircle, LockIcon, LogInIcon, MailIcon, UserIcon } from 'lucide-react'
import { $authenticated, pb } from '@/lib/stores'
import * as v from 'valibot'
import { toast } from './ui/use-toast'
import {
	Dialog,
	DialogContent,
	DialogTrigger,
	DialogDescription,
	DialogHeader,
	DialogTitle,
} from '@/components/ui/dialog'

const honeypot = v.literal('')
const emailSchema = v.pipe(v.string(), v.email('Invalid email address.'))
const passwordSchema = v.pipe(
	v.string(),
	v.minLength(10, 'Password must be at least 10 characters.')
)

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
			'Invalid username. You may use alphanumeric characters, underscores, and hyphens.'
		),
		v.minLength(3, 'Username must be at least 3 characters long.')
	),
	email: emailSchema,
	password: passwordSchema,
	passwordConfirm: passwordSchema,
})

export function UserAuthForm({
	className,
	isFirstRun,
	...props
}: {
	className?: string
	isFirstRun: boolean
}) {
	const [isLoading, setIsLoading] = React.useState<boolean>(false)
	const [isGitHubLoading, setIsGitHubLoading] = React.useState<boolean>(false)
	const [errors, setErrors] = React.useState<Record<string, string | undefined>>({})

	// const searchParams = useSearchParams()

	async function handleSubmit(e: React.FormEvent<HTMLFormElement>) {
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
					let msg = 'Passwords do not match'
					setErrors({ passwordConfirm: msg })
					return
				}
				await pb.admins.create({
					email,
					password,
					passwordConfirm: password,
				})
				await pb.admins.authWithPassword(email, password)
				await pb.collection('users').create({
					username,
					email,
					password,
					passwordConfirm: password,
					role: 'admin',
					verified: true,
				})
				await pb.collection('users').authWithPassword(email, password)
			} else {
				await pb.collection('users').authWithPassword(email, password)
			}
			$authenticated.set(true)
		} catch (e) {
			return toast({
				title: 'Login attempt failed',
				description: 'Please check your credentials and try again',
				variant: 'destructive',
			})
		} finally {
			setIsLoading(false)
		}
	}

	return (
		<div className={cn('grid gap-6', className)} {...props}>
			<form onSubmit={handleSubmit} onChange={() => setErrors({})}>
				<div className="grid gap-2.5">
					{isFirstRun && (
						<div className="grid gap-1 relative">
							<UserIcon className="absolute left-3 top-3 h-4 w-4 text-muted-foreground" />
							<Label className="sr-only" htmlFor="username">
								Username
							</Label>
							<Input
								autoFocus={true}
								id="username"
								name="username"
								required
								placeholder="username"
								type="username"
								autoCapitalize="none"
								autoComplete="username"
								autoCorrect="off"
								disabled={isLoading || isGitHubLoading}
								className="pl-9"
							/>
							{errors?.username && <p className="px-1 text-xs text-red-600">{errors.username}</p>}
						</div>
					)}
					<div className="grid gap-1 relative">
						<MailIcon className="absolute left-3 top-3 h-4 w-4 text-muted-foreground" />
						<Label className="sr-only" htmlFor="email">
							Email
						</Label>
						<Input
							id="email"
							name="email"
							required
							placeholder={isFirstRun ? 'email' : 'name@example.com'}
							type="email"
							autoCapitalize="none"
							autoComplete="email"
							autoCorrect="off"
							disabled={isLoading || isGitHubLoading}
							className="pl-9"
						/>
						{errors?.email && <p className="px-1 text-xs text-red-600">{errors.email}</p>}
					</div>
					<div className="grid gap-1 relative">
						<LockIcon className="absolute left-3 top-3 h-4 w-4 text-muted-foreground" />
						<Label className="sr-only" htmlFor="pass">
							Password
						</Label>
						<Input
							id="pass"
							name="password"
							placeholder="password"
							required
							type="password"
							autoComplete="current-password"
							disabled={isLoading || isGitHubLoading}
							className="pl-9"
						/>
						{errors?.password && <p className="px-1 text-xs text-red-600">{errors.password}</p>}
					</div>
					{isFirstRun && (
						<div className="grid gap-1 relative">
							<LockIcon className="absolute left-3 top-3 h-4 w-4 text-muted-foreground" />
							<Label className="sr-only" htmlFor="pass2">
								Confirm password
							</Label>
							<Input
								id="pass2"
								name="passwordConfirm"
								placeholder="confirm password"
								required
								type="password"
								autoComplete="current-password"
								disabled={isLoading || isGitHubLoading}
								className="pl-9"
							/>
							{errors?.passwordConfirm && (
								<p className="px-1 text-xs text-red-600">{errors.passwordConfirm}</p>
							)}
						</div>
					)}
					<div className="sr-only">
						{/* honeypot */}
						<label htmlFor="name"></label>
						<input id="name" type="text" name="name" tabIndex={-1} />
					</div>
					<button className={cn(buttonVariants())} disabled={isLoading}>
						{isLoading ? (
							<LoaderCircle className="mr-2 h-4 w-4 animate-spin" />
						) : (
							<LogInIcon className="mr-2 h-4 w-4" />
						)}
						{isFirstRun ? 'Create account' : 'Sign in'}
					</button>
				</div>
			</form>
			<div className="relative">
				<div className="absolute inset-0 flex items-center">
					<span className="w-full border-t" />
				</div>
				<div className="relative flex justify-center text-xs uppercase">
					<span className="bg-background px-2 text-muted-foreground">Or continue with</span>
				</div>
			</div>
			<Dialog>
				<DialogTrigger asChild>
					<button
						type="button"
						className={cn(buttonVariants({ variant: 'outline' }))}
						// onClick={async () => {
						// setIsGitHubLoading(true)
						// do stuff
						// setIsGitHubLoading(false)
						// }}
						disabled={isLoading || isGitHubLoading}
					>
						{isGitHubLoading ? (
							<LoaderCircle className="mr-2 h-4 w-4 animate-spin" />
						) : (
							<Github className="mr-2 h-4 w-4" />
						)}{' '}
						Github
					</button>
				</DialogTrigger>
				<DialogContent style={{ maxWidth: 440, width: '90%' }}>
					<DialogHeader>
						<DialogTitle className="mb-2">OAuth 2 / OIDC support</DialogTitle>
						<DialogDescription className="grid gap-3">
							<p>
								Support for OAuth / OIDC (all major providers) will be available in the future. As
								well as an option to disable password auth.
							</p>
							<p>First I need to decide what to do with additional users.</p>
							<p>
								Should systems be shared across all accounts? Or should they be private by default
								with team-based sharing?
							</p>
							<p>Let me know if you have strong opinions either way.</p>
						</DialogDescription>
					</DialogHeader>
				</DialogContent>
			</Dialog>
		</div>
	)
}
