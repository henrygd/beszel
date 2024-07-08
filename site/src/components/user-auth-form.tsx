'use client'

import * as React from 'react'
// import { useSearchParams } from 'next/navigation'
// import { zodResolver } from '@hookform/resolvers/zod'
// import { signIn } from 'next-auth/react'
// import { useForm } from 'react-hook-form'
// import * as z from 'zod'

import { cn } from '@/lib/utils'
import { userAuthSchema } from '@/lib/validations/auth'
import { buttonVariants } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
// import { toast } from '@/components/ui/use-toast'
import { Github, LoaderCircle } from 'lucide-react'

interface UserAuthFormProps extends React.HTMLAttributes<HTMLDivElement> {}

type FormData = z.infer<typeof userAuthSchema>

export function UserAuthForm({ className, ...props }: UserAuthFormProps) {
	const signIn = (s: string) => console.log(s)
	const handleSubmit = (e: React.FormEvent<HTMLFormElement>) => {
		// e.preventDefault()
		signIn('github')
	}

	const errors = {
		email: 'This field is required',
		password: 'This field is required',
	}

	// const {
	// 	register,
	// 	handleSubmit,
	// 	formState: { errors },
	// } = useForm<FormData>({
	// 	resolver: zodResolver(userAuthSchema),
	// })
	const [isLoading, setIsLoading] = React.useState<boolean>(false)
	const [isGitHubLoading, setIsGitHubLoading] = React.useState<boolean>(false)
	// const searchParams = useSearchParams()

	async function onSubmit(data: FormData) {
		setIsLoading(true)

		alert('do pb stuff')

		// const signInResult = await signIn('email', {
		// 	email: data.email.toLowerCase(),
		// 	redirect: false,
		// 	callbackUrl: searchParams?.get('from') || '/dashboard',
		// })

		setIsLoading(false)

		if (!signInResult?.ok) {
			alert('Your sign in request failed. Please try again.')
			// return toast({
			// 	title: 'Something went wrong.',
			// 	description: 'Your sign in request failed. Please try again.',
			// 	variant: 'destructive',
			// })
		}

		// return toast({
		// 	title: 'Check your email',
		// 	description: 'We sent you a login link. Be sure to check your spam too.',
		// })
	}

	return (
		<div className={cn('grid gap-6', className)} {...props}>
			<form onSubmit={handleSubmit}>
				<div className="grid gap-2">
					<div className="grid gap-1">
						<Label className="sr-only" htmlFor="email">
							Email
						</Label>
						<Input
							id="email"
							placeholder="name@example.com"
							type="email"
							autoCapitalize="none"
							autoComplete="email"
							autoCorrect="off"
							disabled={isLoading || isGitHubLoading}
						/>
						{errors?.email && <p className="px-1 text-xs text-red-600">{errors.email.message}</p>}
					</div>
					<button className={cn(buttonVariants())} disabled={isLoading}>
						{isLoading && <LoaderCircle className="mr-2 h-4 w-4 animate-spin" />}
						Sign In with Email
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
			<button
				type="button"
				className={cn(buttonVariants({ variant: 'outline' }))}
				onClick={() => {
					localStorage.setItem('auth', 'true')
					setIsGitHubLoading(true)
					signIn('github')
				}}
				disabled={isLoading || isGitHubLoading}
			>
				{isGitHubLoading ? (
					<LoaderCircle className="mr-2 h-4 w-4 animate-spin" />
				) : (
					<Github className="mr-2 h-4 w-4" />
				)}{' '}
				Github
			</button>
		</div>
	)
}
