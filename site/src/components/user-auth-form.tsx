'use client'

import * as React from 'react'
// import { useSearchParams } from 'next/navigation'
// import { zodResolver } from '@hookform/resolvers/zod'
// import { signIn } from 'next-auth/react'
// import { useForm } from 'react-hook-form'
// import * as z from 'zod'

import { cn } from '@/lib/utils'
import { buttonVariants } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
// import { toast } from '@/components/ui/use-toast'
import { Github, LoaderCircle, LogInIcon } from 'lucide-react'
import { pb } from '@/lib/stores'
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

const LoginSchema = v.object({
	email: v.pipe(v.string(), v.email('Invalid email address.')),
	password: v.pipe(v.string(), v.minLength(10, 'Password must be at least 10 characters long.')),
})

// type LoginData = v.InferOutput<typeof LoginSchema> // { email: string; password: string }

export function UserAuthForm({ className, ...props }: { className?: string }) {
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
			const result = v.safeParse(LoginSchema, data)
			if (!result.success) {
				let errors = {}
				for (const issue of result.issues) {
					// @ts-ignore
					errors[issue.path[0].key] = issue.message
				}
				setErrors(errors)
				return
			}
			const { email, password } = result.output
			let firstRun = true
			if (firstRun) {
				await pb.admins.create({
					email,
					password,
					passwordConfirm: password,
				})
				await pb.admins.authWithPassword(email, password)
			} else {
				await pb.admins.authWithPassword(email, password)
			}
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
			<form onSubmit={handleSubmit}>
				<div className="grid gap-2.5">
					<div className="grid gap-1">
						<Label className="sr-only" htmlFor="email">
							Email
						</Label>
						<Input
							id="email"
							name="email"
							required
							placeholder="name@example.com"
							type="email"
							autoCapitalize="none"
							autoComplete="email"
							autoCorrect="off"
							disabled={isLoading || isGitHubLoading}
						/>
						{errors?.email && <p className="px-1 text-xs text-red-600">{errors.email}</p>}
					</div>
					<div className="grid gap-1">
						<Label className="sr-only" htmlFor="email">
							Email
						</Label>
						<Input
							id="pass"
							name="password"
							placeholder="password"
							required
							type="password"
							autoComplete="current-password"
							disabled={isLoading || isGitHubLoading}
						/>
						{errors?.password && <p className="px-1 text-xs text-red-600">{errors.password}</p>}
					</div>
					<button className={cn(buttonVariants())} disabled={isLoading}>
						{isLoading ? (
							<LoaderCircle className="mr-2 h-4 w-4 animate-spin" />
						) : (
							<LogInIcon className="mr-2 h-4 w-4" />
						)}
						Sign In
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
				<DialogContent className="sm:max-w-[425px]">
					<DialogHeader>
						<DialogTitle>OAuth support coming soon</DialogTitle>
						<DialogDescription>
							OAuth / OpenID with all major providers should be available at 1.0.0.
						</DialogDescription>
					</DialogHeader>
				</DialogContent>
			</Dialog>
		</div>
	)
}
