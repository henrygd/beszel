import { t } from "@lingui/core/macro"
import { MoonStarIcon, SunIcon, SunMoon } from "lucide-react"
import { useTheme } from "@/components/theme-provider"
import { Button } from "@/components/ui/button"
import { Tooltip, TooltipContent, TooltipTrigger } from "./ui/tooltip"
import { Trans } from "@lingui/react/macro"

export function ModeToggle() {
	const { theme, setTheme } = useTheme()

	return (
		<Tooltip>
			<TooltipTrigger asChild>
				<Button
					variant={"ghost"}
					size="icon"
					aria-label={t`Toggle theme`}
					onClick={() => {
						switch (theme) {
							default:
							case "light":
								return setTheme("dark")
							case "dark":
								return setTheme("system")
							case "system":
								return setTheme("light")
						}
					}}
				>
					<SunIcon
						className={`h-[1.2rem] w-[1.2rem] transition-all -rotate-90 ${theme === "light" ? "opacity-100" : "opacity-0"}`}
					/>
					<MoonStarIcon
						className={`absolute h-[1.2rem] w-[1.2rem] transition-all opacity-0 -rotate-90 ${theme === "dark" ? "opacity-100" : "opacity-0"}`}
					/>
					<SunMoon
						className={`absolute h-[1.2rem] w-[1.2rem] transition-all opacity-0 -rotate-90 ${theme === "system" ? "opacity-100" : "opacity-0"}`}
					/>
				</Button>
			</TooltipTrigger>
			<TooltipContent>
				<Trans>Toggle theme</Trans>
			</TooltipContent>
		</Tooltip>
	)
}
