import { cn } from '@/lib/utils'
import { buttonVariants } from '@/components/ui/button'
import { UserAuthForm } from '@/components/user-auth-form'
import { ChevronLeft } from 'lucide-react'

export default function () {
	return (
		<div className="container relative hidden h-screen flex-col items-center justify-center md:grid lg:max-w-none lg:grid-cols-2 lg:px-0">
			<div className="lg:p-8">
				<div className="mx-auto flex w-full flex-col justify-center space-y-6 sm:w-[350px]">
					<div className="flex flex-col space-y-2 text-center">
						{/* <img
							className="mx-auto h-10 w-10 mb-2"
							src="https://upload.wikimedia.org/wikipedia/commons/thumb/d/df/Numelon_Logo.png/240px-Numelon_Logo.png"
							alt=""
						/> */}
						<h1 className="text-2xl font-semibold tracking-tight">Welcome back</h1>
						<p className="text-sm text-muted-foreground">
							Enter your email to sign in to your account
						</p>
					</div>
					<UserAuthForm />
					<p className="px-8 text-center text-sm text-muted-foreground">
						<a href="/register" className="hover:text-brand underline underline-offset-4">
							Don&apos;t have an account? Sign Up
						</a>
					</p>
				</div>
			</div>
			<div className="relative hidden h-full flex-col bg-muted p-10 text-white dark:border-r lg:flex">
				<div
					className="absolute inset-0 bg-background bg-cover opacity-80"
					style={{
						backgroundImage: `url(https://directus.cloud/assets/waves.2b156907.svg)`,
					}}
				></div>
				<div className="relative z-20 flex gap-2 items-center text-lg font-medium ml-auto">
					placeholder
					<img
						className={'w-6 h-6'}
						src="https://upload.wikimedia.org/wikipedia/commons/thumb/d/df/Numelon_Logo.png/240px-Numelon_Logo.png"
						alt=""
					/>
				</div>
				{/* <div className="relative z-20 mt-auto">
					<blockquote className="space-y-2">
						<p className="text-lg">
							“This library has saved me countless hours of work and helped me deliver stunning
							designs to my clients faster than ever before.”
						</p>
						<footer className="text-sm">Sofia Davis</footer>
					</blockquote>
				</div> */}
			</div>
		</div>
	)
}
