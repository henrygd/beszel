import { t } from "@lingui/core/macro"
import { MoonStarIcon, SunIcon } from "lucide-react"
import { useTheme } from "@/components/theme-provider"
import { Button } from "@/components/ui/button"

export function ModeToggle() {
	const { theme, setTheme } = useTheme()

	return (
		<Button
			variant={"ghost"}
			size="icon"
			aria-label={t`Toggle theme`}
			onClick={() => setTheme(theme === "dark" ? "light" : "dark")}
		>
			<SunIcon className="h-[1.2rem] w-[1.2rem] transition-all -rotate-90 dark:opacity-0 dark:rotate-0" />
			<MoonStarIcon className="absolute h-[1.2rem] w-[1.2rem] transition-all opacity-0 -rotate-90 dark:opacity-100 dark:rotate-0" />
		</Button>
	)
}
