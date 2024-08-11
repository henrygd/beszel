import { LoaderCircle, MailIcon, SendHorizonalIcon } from 'lucide-react'
import { Input } from '../ui/input'
import { Label } from '../ui/label'
import { useCallback, useState } from 'react'
import { toast } from '../ui/use-toast'
import { buttonVariants } from '../ui/button'
import { cn } from '@/lib/utils'
import { pb } from '@/lib/stores'
import { Dialog, DialogHeader } from '../ui/dialog'
import { DialogContent, DialogTrigger, DialogTitle } from '../ui/dialog'

const showLoginFaliedToast = () => {
	toast({
		title: 'Login attempt failed',
		description: 'Please check your credentials and try again',
		variant: 'destructive',
	})
}

export default function ForgotPassword() {
	const [isLoading, setIsLoading] = useState<boolean>(false)
	const [email, setEmail] = useState('')

	const handleSubmit = useCallback(
		async (e: React.FormEvent<HTMLFormElement>) => {
			e.preventDefault()
			setIsLoading(true)
			try {
				// console.log(email)
				await pb.collection('users').requestPasswordReset(email)
				toast({
					title: 'Password reset request received',
					description: `Check ${email} for a reset link.`,
				})
			} catch (e) {
				showLoginFaliedToast()
			} finally {
				setIsLoading(false)
				setEmail('')
			}
		},
		[email]
	)

	return (
		<>
			<form onSubmit={handleSubmit}>
				<div className="grid gap-3">
					<div className="grid gap-1 relative">
						<MailIcon className="absolute left-3 top-3 h-4 w-4 text-muted-foreground" />
						<Label className="sr-only" htmlFor="email">
							Email
						</Label>
						<Input
							value={email}
							onChange={(e) => setEmail(e.target.value)}
							id="email"
							name="email"
							required
							placeholder="name@example.com"
							type="email"
							autoCapitalize="none"
							autoComplete="email"
							autoCorrect="off"
							disabled={isLoading}
							className="pl-9"
						/>
					</div>
					<button className={cn(buttonVariants())} disabled={isLoading}>
						{isLoading ? (
							<LoaderCircle className="mr-2 h-4 w-4 animate-spin" />
						) : (
							<SendHorizonalIcon className="mr-2 h-4 w-4" />
						)}
						Reset password
					</button>
				</div>
			</form>
			<Dialog>
				<DialogTrigger asChild>
					<button className="text-sm mx-auto hover:text-brand underline underline-offset-4 opacity-70 hover:opacity-100 transition-opacity">
						Command line instructions
					</button>
				</DialogTrigger>
				<DialogContent className="max-w-[33em]">
					<DialogHeader>
						<DialogTitle>Command line instructions</DialogTitle>
					</DialogHeader>
					<p className="text-primary/70 text-[0.95em] leading-relaxed">
						If you've lost the password to your admin account, you may reset it using the following
						command.
					</p>
					<p className="text-primary/70 text-[0.95em] leading-relaxed">
						Then log into the backend and reset your user account password in the users table.
					</p>
					<code className="bg-muted rounded-sm py-0.5 px-2.5 mr-auto text-sm">
						beszel admin update youremail@example.com newpassword
					</code>
				</DialogContent>
			</Dialog>
		</>
	)
}
