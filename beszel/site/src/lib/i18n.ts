import i18n from "i18next"
import { initReactI18next } from "react-i18next"

import en from "../locales/en/translation.json"
import es from "../locales/es/translation.json"
import fr from "../locales/fr/translation.json"
import de from "../locales/de/translation.json"
import ru from "../locales/ru/translation.json"
import zhHans from "../locales/zh-CN/translation.json"
import zhHant from "../locales/zh-HK/translation.json"

// Custom language detector to use localStorage
const languageDetector: any = {
	type: "languageDetector",
	async: true,
	detect: (callback: (lng: string) => void) => {
		const savedLanguage = localStorage.getItem("i18nextLng")
		const fallbackLanguage = (() => {
			switch (navigator.language) {
				case "zh-CN":
				case "zh-SG":
				case "zh-MY":
				case "zh":
				case "zh-Hans":
					return "zh-CN"
				case "zh-HK":
				case "zh-TW":
				case "zh-MO":
				case "zh-Hant":
					return "zh-HK"
				default:
					return navigator.language
			}
		})()
		callback(savedLanguage || fallbackLanguage)
	},
	init: () => {},
	cacheUserLanguage: (lng: string) => {
		localStorage.setItem("i18nextLng", lng)
	},
}

i18n
	.use(languageDetector)
	.use(initReactI18next)
	.init({
		resources: {
			en: { translation: en },
			es: { translation: es },
			fr: { translation: fr },
			de: { translation: de },
			ru: { translation: ru },
			// Chinese (Simplified)
			"zh-CN": { translation: zhHans },
			// Chinese (Traditional)
			"zh-HK": { translation: zhHant },
		},
		fallbackLng: "en",
		interpolation: {
			escapeValue: false,
		},
	})

export { i18n }
