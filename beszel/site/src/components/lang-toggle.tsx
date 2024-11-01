import { useEffect } from "react"
import { LanguagesIcon } from "lucide-react"

import { Button } from "@/components/ui/button"
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from "@/components/ui/dropdown-menu"
import { useTranslation } from "react-i18next"
import languages from "../lib/languages.json"
import { cn } from "@/lib/utils"
import { setLang } from "@/lib/i18n"

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
			<DropdownMenuContent className="grid grid-cols-2">
				{languages.map(({ lang, label, e }) => (
					<DropdownMenuItem
						key={lang}
						className={cn("px-3 flex gap-2.5", lang === i18n.language ? "font-bold" : "")}
						onClick={() => setLang(lang)}
					>
						<span>{e}</span> {label}
					</DropdownMenuItem>
				))}
			</DropdownMenuContent>
		</DropdownMenu>
	)
}
