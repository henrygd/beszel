import { t } from "@lingui/core/macro"
import { CopyIcon, EyeIcon, EyeOffIcon } from "lucide-react"
import { useState } from "react"
import { Button } from "@/components/ui/button"
import { copyToClipboard } from "@/lib/utils"

/** Reduce accidental disclosure during screen sharing; this is not an access control. */
export function TokenValue({ token, label, disabled = false }: { token: string; label: string; disabled?: boolean }) {
	const [revealedToken, setRevealedToken] = useState<string | null>(null)
	const revealed = !disabled && revealedToken === token

	return (
		<fieldset className="flex items-center gap-2 min-w-0">
			<legend className="sr-only">{label}</legend>
			<span className="font-mono text-sm min-w-0 flex-1 overflow-x-auto whitespace-nowrap" aria-hidden={!revealed}>
				{revealed ? token : "••••••••••••"}
			</span>
			{!revealed && <span className="sr-only">{t`Token hidden`}</span>}
			<Button
				type="button"
				variant="ghost"
				size="icon"
				className="shrink-0"
				disabled={disabled}
				aria-label={revealed ? t`Hide token` : t`Reveal token`}
				title={revealed ? t`Hide token` : t`Reveal token`}
				aria-pressed={revealed}
				onClick={() => setRevealedToken(revealed ? null : token)}
			>
				{revealed ? (
					<EyeOffIcon className="size-4" aria-hidden="true" />
				) : (
					<EyeIcon className="size-4" aria-hidden="true" />
				)}
			</Button>
			<Button
				type="button"
				variant="ghost"
				size="icon"
				className="shrink-0"
				disabled={disabled}
				aria-label={t`Copy token`}
				title={t`Copy token`}
				onClick={() => copyToClipboard(token)}
			>
				<CopyIcon className="size-4" aria-hidden="true" />
			</Button>
		</fieldset>
	)
}
