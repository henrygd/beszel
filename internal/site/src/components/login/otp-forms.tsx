import { useCallback, useState } from "react"
import { pb } from "@/lib/api"
import { $authenticated } from "@/lib/stores"
import { InputOTP, InputOTPGroup, InputOTPSlot } from "@/components/ui/otp"
import { Trans } from "@lingui/react/macro"
import { showLoginFaliedToast } from "./auth-form"
import { cn } from "@/lib/utils"
import { MailIcon, LoaderCircle, SendHorizonalIcon } from "lucide-react"
import { Label } from "../ui/label"
import { buttonVariants } from "../ui/button"
import { Input } from "../ui/input"
import { $router } from "../router"

export function OtpInputForm({ otpId, mfaId }: { otpId: string; mfaId: string }) {
	const [value, setValue] = useState("")

	if (value.length === 6) {
		pb.collection("users")
			.authWithOTP(otpId, value, { mfaId })
			.then(() => {
				$router.open("/")
				$authenticated.set(true)
			})
			.catch((err) => {
				showLoginFaliedToast(err.message)
			})
	}

	return (
		<div className="grid gap-3 items-center justify-center">
			<InputOTP maxLength={6} value={value} onChange={setValue} autoFocus>
				<InputOTPGroup>
					{Array.from({ length: 6 }).map((_, i) => (
						<InputOTPSlot key={i} index={i} />
					))}
				</InputOTPGroup>
			</InputOTP>
			<div className="text-center text-sm text-muted-foreground">
				<Trans>Enter your one-time password.</Trans>
			</div>
		</div>
	)
}

export function OtpRequestForm() {
	const [isLoading, setIsLoading] = useState<boolean>(false)
	const [email, setEmail] = useState("")
	const [otpId, setOtpId] = useState<string | undefined>()

	const handleSubmit = useCallback(
		async (e: React.FormEvent<HTMLFormElement>) => {
			e.preventDefault()
			setIsLoading(true)
			try {
				// console.log(email)
				const { otpId } = await pb.collection("users").requestOTP(email)
				setOtpId(otpId)
			} catch (e: any) {
				showLoginFaliedToast(e?.message)
			} finally {
				setIsLoading(false)
				setEmail("")
			}
		},
		[email]
	)

	if (otpId) {
		return <OtpInputForm otpId={otpId} mfaId={""} />
	}

	return (
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
					<Trans>Request OTP</Trans>
				</button>
			</div>
		</form>
	)
}
