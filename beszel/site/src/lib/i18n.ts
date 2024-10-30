import i18n from "i18next"
import { initReactI18next } from "react-i18next"

// Custom language detector to use localStorage
const languageDetector: any = {
	type: "languageDetector",
	async: true,
	detect: (callback: (lng: string) => void) => {
		const savedLanguage = localStorage.getItem("i18nextLng")
		const zhVariantMap: Record<string, string> = {
			"zh-CN": "zh-CN",
			"zh-SG": "zh-CN",
			"zh-MY": "zh-CN",
			zh: "zh-CN",
			"zh-Hans": "zh-CN",
			"zh-HK": "zh-HK",
			"zh-TW": "zh-HK",
			"zh-MO": "zh-HK",
			"zh-Hant": "zh-HK",
		}
		const fallbackLanguage = zhVariantMap[navigator.language] || navigator.language
		callback(savedLanguage || fallbackLanguage)
	},
	init: () => {},
	cacheUserLanguage: (lng: string) => {
		localStorage.setItem("i18nextLng", lng)
	},
}

// Function to dynamically load translation files
async function loadMessages(locale: string) {
	try {
		const translation = await import(`../locales/${locale}/translation.json`)
		return translation.default
	} catch (error) {
		console.error(`Error loading ${locale}`, error)
		// Fallback to English if translation fails to load
		const enTranslation = await import(`../locales/en/translation.json`)
		return enTranslation.default
	}
}

i18n
	.use(languageDetector)
	.use(initReactI18next)
	.init({
		resources: {}, // Start with empty resources
		fallbackLng: "en",
		interpolation: {
			escapeValue: false,
		},
	})

// Function to dynamically activate a language
export async function setLang(locale: string) {
	const messages = await loadMessages(locale)
	i18n.addResourceBundle(locale, "translation", messages)
	await i18n.changeLanguage(locale)
}

// Initialize with detected/saved language
const initialLanguage = localStorage.getItem("i18nextLng") || navigator.language
setLang(initialLanguage)

export { i18n }
