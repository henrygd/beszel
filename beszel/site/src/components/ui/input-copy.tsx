import { copyToClipboard } from "@/lib/utils"
import { Input } from "./input"
import { Trans } from "@lingui/react/macro"
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from "./tooltip"
import { CopyIcon } from "lucide-react"
import { Button } from "./button"

export function InputCopy({ value, id, name }: { value: string; id: string; name: string }) {
	return (
		<div className="relative">
			<Input readOnly id={id} name={name} value={value} required></Input>
			<div
				className={
					"h-6 w-24 bg-linear-to-r rtl:bg-linear-to-l from-transparent to-background to-65% absolute top-2 end-1 pointer-events-none"
				}
			></div>
			<TooltipProvider delayDuration={100} disableHoverableContent>
				<Tooltip disableHoverableContent={true}>
					<TooltipTrigger asChild>
						<Button
							type="button"
							variant={"link"}
							className="absolute end-0 top-0"
							onClick={() => copyToClipboard(value)}
						>
							<CopyIcon className="size-4" />
						</Button>
					</TooltipTrigger>
					<TooltipContent>
						<p>
							<Trans>Click to copy</Trans>
						</p>
					</TooltipContent>
				</Tooltip>
			</TooltipProvider>
		</div>
	)
}
