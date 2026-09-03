import { useStore } from "@nanostores/react"
import { createContext, useContext, useEffect, useState } from "react"
import { pb, saveUserSettings } from "@/lib/api"
import { $userSettings } from "@/lib/stores"
import { THEME_STORAGE_KEY } from "@/lib/utils"

export type Theme = "dark" | "light" | "system"

type ThemeProviderProps = {
	children: React.ReactNode
	defaultTheme?: Theme
	storageKey?: string
}

type ThemeProviderState = {
	theme: Theme
	setTheme: (theme: Theme) => void
}

const initialState: ThemeProviderState = {
	theme: "system",
	setTheme: () => null,
}

const ThemeProviderContext = createContext<ThemeProviderState>(initialState)

export function ThemeProvider({
	children,
	defaultTheme = "system",
	storageKey = THEME_STORAGE_KEY,
	...props
}: ThemeProviderProps) {
	const [theme, setTheme] = useState<Theme>(() => (localStorage.getItem(storageKey) as Theme) || defaultTheme)
	const { theme: savedTheme } = useStore($userSettings, { keys: ["theme"] })

	useEffect(() => {
		const root = window.document.documentElement

		root.classList.remove("light", "dark")

		if (theme === "system") {
			const systemTheme = window.matchMedia("(prefers-color-scheme: dark)").matches ? "dark" : "light"

			root.classList.add(systemTheme)
			return
		}

		root.classList.add(theme)
	}, [theme])

	// an undefined value is ignored so logging out, which clears user settings,
	// doesn't reset the theme
	useEffect(() => {
		if (savedTheme && savedTheme !== theme) {
			localStorage.setItem(storageKey, savedTheme)
			setTheme(savedTheme)
		}
	}, [savedTheme])

	const value = {
		theme,
		setTheme: (theme: Theme) => {
			localStorage.setItem(storageKey, theme)
			setTheme(theme)
			// local storage alone is lost in private windows and on other devices
			if (pb.authStore.isValid) {
				$userSettings.setKey("theme", theme)
				saveUserSettings({ theme }).catch((e) => console.error("save theme", e))
			}
		},
	}

	return (
		<ThemeProviderContext.Provider {...props} value={value}>
			{children}
		</ThemeProviderContext.Provider>
	)
}

export const useTheme = () => useContext(ThemeProviderContext)
