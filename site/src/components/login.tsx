import { UserAuthForm } from '@/components/user-auth-form'
import { Logo } from './logo'
import { useEffect, useState } from 'react'
import { pb } from '@/lib/stores'

export default function () {
	const [isFirstRun, setFirstRun] = useState(false)

	useEffect(() => {
		document.title = 'Login / Qoma'

		pb.send('/api/qoma/first-run', {}).then(({ firstRun }) => {
			setFirstRun(firstRun)
		})
	}, [])

	return (
		<div className="relative h-screen grid lg:max-w-none lg:px-0">
			<div className="grid items-center py-12">
				<div className="grid gap-5 w-full px-4 max-w-[22em] mx-auto">
					<div className="text-center">
						<h1 className="mb-3">
							<Logo className="h-7 fill-foreground mx-auto" />
							<span className="sr-only">Qoma</span>
						</h1>
						<p className="text-sm text-muted-foreground">
							{isFirstRun ? 'Please create your admin account' : 'Please sign in to your account'}
						</p>
					</div>
					<UserAuthForm isFirstRun={isFirstRun} />
					<p className="text-center text-sm opacity-70 hover:opacity-100 transition-opacity">
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
			{/* <div className="relative hidden h-full bg-primary lg:block">
				<img
					className="absolute inset-0 h-full w-full object-cover bg-primary"
					src="/penguin-and-egg.avif"
				></img>
			</div> */}
		</div>
	)
}
