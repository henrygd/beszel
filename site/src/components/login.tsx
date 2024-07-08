import { Link } from 'wouter-preact'

import { cn } from '@/lib/utils'
import { buttonVariants } from '@/components/ui/button'
import { UserAuthForm } from '@/components/user-auth-form'
import { ChevronLeft } from 'lucide-react'

export default function LoginPage() {
	return (
		<div className="container relative hidden h-screen flex-col items-center justify-center md:grid lg:max-w-none lg:grid-cols-2 lg:px-0">
			<div className="lg:p-8">
				<div className="mx-auto flex w-full flex-col justify-center space-y-6 sm:w-[350px]">
					<div className="flex flex-col space-y-2 text-center">
						<ChevronLeft className="mx-auto h-6 w-6" />
						<h1 className="text-2xl font-semibold tracking-tight">Welcome back</h1>
						<p className="text-sm text-muted-foreground">
							Enter your email to sign in to your account
						</p>
					</div>
					<UserAuthForm />
					<p className="px-8 text-center text-sm text-muted-foreground">
						<Link href="/register" className="hover:text-brand underline underline-offset-4">
							Don&apos;t have an account? Sign Up
						</Link>
					</p>
				</div>
			</div>
			<div class="relative hidden h-full flex-col bg-muted p-10 text-white dark:border-r lg:flex">
				<div
					class="absolute inset-0 bg-slate-900 bg-cover opacity-80"
					style={{
						backgroundImage: `url(https://directus.cloud/assets/waves.2b156907.svg)`,
					}}
				></div>
				<div class="relative z-20 flex gap-2 items-center text-lg font-medium ml-auto">
					Melon
					<svg
						xmlns="http://www.w3.org/2000/svg"
						viewBox="0 0 24 24"
						fill="none"
						stroke="currentColor"
						stroke-width="2"
						stroke-linecap="round"
						stroke-linejoin="round"
						class="h-6 w-6"
					>
						<path d="M15 6v12a3 3 0 1 0 3-3H6a3 3 0 1 0 3 3V6a3 3 0 1 0-3 3h12a3 3 0 1 0-3-3"></path>
					</svg>
				</div>
				{/* <div class="relative z-20 mt-auto">
					<blockquote class="space-y-2">
						<p class="text-lg">
							“This library has saved me countless hours of work and helped me deliver stunning
							designs to my clients faster than ever before.”
						</p>
						<footer class="text-sm">Sofia Davis</footer>
					</blockquote>
				</div> */}
			</div>
		</div>
	)
}
