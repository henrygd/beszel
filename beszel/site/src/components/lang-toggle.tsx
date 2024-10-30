import { useEffect } from "react"
import { LanguagesIcon } from "lucide-react"

import { Button } from "@/components/ui/button"
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from "@/components/ui/dropdown-menu"
import { useTranslation } from "react-i18next"
import languages from "../lib/languages.json"
import { cn } from "@/lib/utils"

export function LangToggle() {
	const { i18n } = useTranslation()

	useEffect(() => {
		document.documentElement.lang = i18n.language
	}, [i18n.language])

	return (
		<DropdownMenu>
			<DropdownMenuTrigger asChild>
				<Button variant={"ghost"} size="icon" className="hidden 450:flex">
					<LanguagesIcon className="absolute h-[1.2rem] w-[1.2rem] light:opacity-85" />
					<span className="sr-only">Language</span>
				</Button>
			</DropdownMenuTrigger>
			<DropdownMenuContent>
				{languages.map(({ lang, label }) => (
					<DropdownMenuItem
						key={lang}
						className={cn("pl-4", lang === i18n.language ? "font-bold" : "")}
						onClick={() => i18n.changeLanguage(lang)}
					>
						{label}
					</DropdownMenuItem>
				))}
			</DropdownMenuContent>
		</DropdownMenu>
	)
}
