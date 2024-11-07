import { LanguagesIcon } from "lucide-react"

import { Button } from "@/components/ui/button"
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from "@/components/ui/dropdown-menu"
import languages from "@/lib/languages"
import { cn } from "@/lib/utils"
import { useLingui } from "@lingui/react"
import { dynamicActivate } from "@/lib/i18n"

export function LangToggle() {
	const { i18n } = useLingui()

	return (
		<DropdownMenu>
			<DropdownMenuTrigger asChild>
				<Button variant={"ghost"} size="icon" className="hidden 450:flex">
					<LanguagesIcon className="absolute h-[1.2rem] w-[1.2rem] light:opacity-85" />
					<span className="sr-only">Language</span>
				</Button>
			</DropdownMenuTrigger>
			<DropdownMenuContent className="grid grid-cols-3">
				{languages.map(({ lang, label, e }) => (
					<DropdownMenuItem
						key={lang}
						className={cn("px-2.5 flex gap-2.5", lang === i18n.locale && "font-semibold")}
						onClick={() => dynamicActivate(lang)}
					>
						<span>{e}</span> {label}
					</DropdownMenuItem>
				))}
			</DropdownMenuContent>
		</DropdownMenu>
	)
}
