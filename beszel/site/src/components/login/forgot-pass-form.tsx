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
import { useTranslation } from 'react-i18next'

const showLoginFaliedToast = () => {
	toast({
		title: 'Login attempt failed',
		description: 'Please check your credentials and try again',
		variant: 'destructive',
	})
}

export default function ForgotPassword() {
	const { t } = useTranslation()
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
						{t('auth.reset_password')}
					</button>
				</div>
			</form>
			<Dialog>
				<DialogTrigger asChild>
					<button className="text-sm mx-auto hover:text-brand underline underline-offset-4 opacity-70 hover:opacity-100 transition-opacity">
						{t('auth.command_line_instructions')}
					</button>
				</DialogTrigger>
				<DialogContent className="max-w-[33em]">
					<DialogHeader>
						<DialogTitle>{t('auth.command_line_instructions')}</DialogTitle>
					</DialogHeader>
					<p className="text-primary/70 text-[0.95em] leading-relaxed">
						{t('auth.command_1')}
					</p>
					<p className="text-primary/70 text-[0.95em] leading-relaxed">
						{t('auth.command_2')}
					</p>
					<code className="bg-muted rounded-sm py-0.5 px-2.5 mr-auto text-sm">
						beszel admin update youremail@example.com newpassword
					</code>
				</DialogContent>
			</Dialog>
		</>
	)
}
