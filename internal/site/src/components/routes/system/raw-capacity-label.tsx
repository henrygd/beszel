import { t } from "@lingui/core/macro"
import { InfoIcon } from "lucide-react"
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip"

export function RawCapacityLabel({ label = t`Raw capacity` }: { label?: string }) {
	return (
		<span className="inline-flex items-center gap-1">
			{label}
			<Tooltip>
				<TooltipTrigger asChild>
					<button
						type="button"
						aria-label={t`About raw capacity`}
						className="inline-flex rounded-sm text-muted-foreground hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
					>
						<InfoIcon className="size-3.5" aria-hidden="true" />
					</button>
				</TooltipTrigger>
				<TooltipContent className="max-w-64">
					{t`Physical device space. True usable capacity is unknown. Pool disk usage alerts are disabled.`}
				</TooltipContent>
			</Tooltip>
		</span>
	)
}
