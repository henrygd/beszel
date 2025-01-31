import { LoaderCircle, MailIcon, SendHorizonalIcon } from "lucide-react"
import { Input } from "../ui/input"
import { Label } from "../ui/label"
import { useCallback, useState } from "react"
import { toast } from "../ui/use-toast"
import { buttonVariants } from "../ui/button"
import { cn } from "@/lib/utils"
import { pb } from "@/lib/stores"
import { Dialog, DialogHeader } from "../ui/dialog"
import { DialogContent, DialogTrigger, DialogTitle } from "../ui/dialog"
import { t, Trans } from "@lingui/macro"

const showLoginFaliedToast = () => {
	toast({
		title: t`Login attempt failed`,
		description: t`Please check your credentials and try again`,
		variant: "destructive",
	})
}

export default function ForgotPassword() {
	const [isLoading, setIsLoading] = useState<boolean>(false)
	const [email, setEmail] = useState("")

	const handleSubmit = useCallback(
		async (e: React.FormEvent<HTMLFormElement>) => {
			e.preventDefault()
			setIsLoading(true)
			try {
				// console.log(email)
				await pb.collection("users").requestPasswordReset(email)
				toast({
					title: t`Password reset request received`,
					description: t`Check ${email} for a reset link.`,
				})
			} catch (e) {
				showLoginFaliedToast()
			} finally {
				setIsLoading(false)
				setEmail("")
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
							<Trans>Email</Trans>
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
							className="ps-9"
						/>
					</div>
					<button className={cn(buttonVariants())} disabled={isLoading}>
						{isLoading ? (
							<LoaderCircle className="me-2 h-4 w-4 animate-spin" />
						) : (
							<SendHorizonalIcon className="me-2 h-4 w-4" />
						)}
						<Trans>Reset Password</Trans>
					</button>
				</div>
			</form>
			<Dialog>
				<DialogTrigger asChild>
					<button className="text-sm mx-auto hover:text-brand underline underline-offset-4 opacity-70 hover:opacity-100 transition-opacity">
						<Trans>Command line instructions</Trans>
					</button>
				</DialogTrigger>
				<DialogContent className="max-w-[41em]">
					<DialogHeader>
						<DialogTitle>
							<Trans>Command line instructions</Trans>
						</DialogTitle>
					</DialogHeader>
					<p className="text-primary/70 text-[0.95em] leading-relaxed">
						<Trans>
							If you've lost the password to your admin account, you may reset it using the following command.
						</Trans>
					</p>
					<p className="text-primary/70 text-[0.95em] leading-relaxed">
						<Trans>Then log into the backend and reset your user account password in the users table.</Trans>
					</p>
					<code className="bg-muted rounded-sm py-0.5 px-2.5 me-auto text-sm">
						./beszel superuser upsert user@example.com password
					</code>
					<code className="bg-muted rounded-sm py-0.5 px-2.5 me-auto text-sm">
						docker exec beszel /beszel superuser upsert name@example.com password
					</code>
				</DialogContent>
			</Dialog>
		</>
	)
}
