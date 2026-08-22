import { createContext, useContext, useEffect, useState } from "react"

type Theme = "dark" | "light" | "system"

const themes: Theme[] = ["light", "dark", "system"]

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
	storageKey = "ui-theme",
	...props
}: ThemeProviderProps) {
	const [theme, setTheme] = useState<Theme>(() => {
		try {
			const savedTheme = localStorage.getItem(storageKey)
			return savedTheme && themes.includes(savedTheme as Theme) ? (savedTheme as Theme) : defaultTheme
		} catch {
			return defaultTheme
		}
	})

	useEffect(() => {
		const root = window.document.documentElement
		const media = window.matchMedia("(prefers-color-scheme: dark)")

		const applyTheme = () => {
			root.classList.remove("light", "dark")
			root.classList.add(theme === "system" ? (media.matches ? "dark" : "light") : theme)
		}

		applyTheme()
		if (theme !== "system") return

		media.addEventListener("change", applyTheme)
		return () => media.removeEventListener("change", applyTheme)
	}, [theme])

	const value = {
		theme,
		setTheme: (theme: Theme) => {
			try {
				localStorage.setItem(storageKey, theme)
			} catch {
				// Theme changes still apply for this session when storage is blocked.
			}
			setTheme(theme)
		},
	}

	return (
		<ThemeProviderContext.Provider {...props} value={value}>
			{children}
		</ThemeProviderContext.Provider>
	)
}

export const useTheme = () => useContext(ThemeProviderContext)
