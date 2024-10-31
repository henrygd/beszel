import { LaptopIcon, MoonStarIcon, SunIcon } from "lucide-react"

import { Button } from "@/components/ui/button"
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from "@/components/ui/dropdown-menu"
import { useTheme } from "@/components/theme-provider"
import { useTranslation } from "react-i18next"
import { cn } from "@/lib/utils"

export function ModeToggle() {
	const { t } = useTranslation()
	const { theme, setTheme } = useTheme()

	const options = [
		{
			theme: "light",
			Icon: SunIcon,
			label: t("themes.light"),
		},
		{
			theme: "dark",
			Icon: MoonStarIcon,
			label: t("themes.dark"),
		},
		{
			theme: "system",
			Icon: LaptopIcon,
			label: t("themes.system"),
		},
	]

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
				{options.map((opt) => {
					const selected = opt.theme === theme
					return (
						<DropdownMenuItem
							key={opt.theme}
							className={cn("px-2.5", selected ? "font-semibold" : "")}
							onClick={() => setTheme(opt.theme as "dark" | "light" | "system")}
						>
							<opt.Icon className={cn("me-2 h-4 w-4 opacity-80", selected && "opacity-100")} />
							{opt.label}
						</DropdownMenuItem>
					)
				})}
			</DropdownMenuContent>
		</DropdownMenu>
	)
}
