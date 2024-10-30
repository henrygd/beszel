import { LaptopIcon, MoonStarIcon, SunIcon } from "lucide-react"

import { Button } from "@/components/ui/button"
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from "@/components/ui/dropdown-menu"
import { useTheme } from "@/components/theme-provider"
import { useTranslation } from "react-i18next"

export function ModeToggle() {
	const { t } = useTranslation()
	const { setTheme } = useTheme()

	return (
		<DropdownMenu>
			<DropdownMenuTrigger asChild>
				<Button variant={"ghost"} size="icon">
					<SunIcon className="h-[1.2rem] w-[1.2rem] dark:opacity-0" />
					<MoonStarIcon className="absolute h-[1.2rem] w-[1.2rem] opacity-0 dark:opacity-100" />
					<span className="sr-only">{t("themes.toggle_theme")}</span>
				</Button>
			</DropdownMenuTrigger>
			<DropdownMenuContent>
				<DropdownMenuItem onClick={() => setTheme("light")}>
					<SunIcon className="mr-2.5 h-4 w-4" />
					{t("themes.light")}
				</DropdownMenuItem>
				<DropdownMenuItem onClick={() => setTheme("dark")}>
					<MoonStarIcon className="mr-2.5 h-4 w-4" />
					{t("themes.dark")}
				</DropdownMenuItem>
				<DropdownMenuItem onClick={() => setTheme("system")}>
					<LaptopIcon className="mr-2.5 h-4 w-4" />
					{t("themes.system")}
				</DropdownMenuItem>
			</DropdownMenuContent>
		</DropdownMenu>
	)
}
