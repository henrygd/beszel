import { UserAuthForm } from '@/components/user-auth-form'
import { Logo } from './logo'

export default function () {
	return (
		<div className="relative h-screen grid lg:max-w-none lg:grid-cols-2 lg:px-0">
			<div className="grid items-center">
				<div className="flex flex-col justify-center space-y-6 w-full px-4 max-w-[22em] mx-auto">
					<div className="text-center">
						<h1 className="mb-4">
							<Logo className="h-6 fill-foreground mx-auto" />
							<div className="sr-only">Qoma</div>
						</h1>
						<p className="text-sm text-muted-foreground">
							Enter your email to sign in to your account
						</p>
					</div>
					<UserAuthForm />
					<p className="px-8 text-center text-sm text-muted-foreground">
						{/* todo: add forgot password section to readme and link to section
						    reset w/ command or link to pb reset */}
						<a
							href="https://github.com/henrygd/qoma"
							className="hover:text-brand underline underline-offset-4"
						>
							Forgot password?
						</a>
					</p>
				</div>
			</div>
			<div className="relative hidden h-full bg-primary lg:block">
				<img
					className="absolute inset-0 h-full w-full object-cover bg-primary"
					src="/penguin-and-egg.avif"
				></img>
			</div>
		</div>
	)
}
