import { Trans, useLingui } from "@lingui/react/macro"
import { LanguagesIcon } from "lucide-react"
import { Button } from "@/components/ui/button"
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from "@/components/ui/dropdown-menu"
import { dynamicActivate } from "@/lib/i18n"
import languages from "@/lib/languages"
import { cn } from "@/lib/utils"
import { Tooltip, TooltipContent, TooltipTrigger } from "./ui/tooltip"

export function LangToggle() {
	const { i18n } = useLingui()

	const LangTrans = <Trans>Language</Trans>

	return (
		<DropdownMenu>
			<DropdownMenuTrigger>
				<Tooltip>
					<TooltipTrigger asChild>
						<Button variant={"ghost"} size="icon" className="hidden sm:flex">
							<LanguagesIcon className="absolute h-[1.2rem] w-[1.2rem] light:opacity-85" />
							<span className="sr-only">{LangTrans}</span>
						</Button>
					</TooltipTrigger>
					<TooltipContent>{LangTrans}</TooltipContent>
				</Tooltip>
			</DropdownMenuTrigger>
			<DropdownMenuContent className="grid grid-cols-3">
				{languages.map(([lang, label, e]) => (
					<DropdownMenuItem
						key={lang}
						className={cn("px-2.5 flex gap-2.5 cursor-pointer", lang === i18n.locale && "bg-accent/70 font-medium")}
						onClick={() => dynamicActivate(lang)}
					>
						<span>{e}</span> {label}
					</DropdownMenuItem>
				))}
			</DropdownMenuContent>
		</DropdownMenu>
	)
}
