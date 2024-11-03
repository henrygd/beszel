import { $direction } from "./stores"
import { i18n } from "@lingui/core"
import type { Messages } from "@lingui/core"
import languages from "@/lib/languages.json"
import { detect, fromUrl, fromStorage, fromNavigator } from "@lingui/detect-locale"
import { messages as enMessages } from "../locales/en/en.ts"

console.log(languages)

// let locale = detect(fromUrl("lang"), fromStorage("lang"), fromNavigator(), "en")
let locale = detect(fromStorage("lang"), fromNavigator(), "en")

// log if dev
if (import.meta.env.DEV) {
	console.log("detected locale", locale)
}

// activates locale
function activateLocale(locale: string, messages: Messages = enMessages) {
	i18n.load(locale, messages)
	i18n.activate(locale)
	document.documentElement.lang = locale
	$direction.set(locale.startsWith("ar") ? "rtl" : "ltr")
}

// dynamically loads translations for the given locale
export async function dynamicActivate(locale: string) {
	try {
		const { messages }: { messages: Messages } = await import(`../locales/${locale}/${locale}.ts`)
		activateLocale(locale, messages)
		localStorage.setItem("lang", locale)
	} catch (error) {
		console.error(`Error loading ${locale}`, error)
		activateLocale("en")
	}
}

// handle zh variants
if (locale?.startsWith("zh-")) {
	// map zh variants to zh-CN
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
	dynamicActivate(zhVariantMap[locale] || "zh-CN")
} else {
	locale = (locale || "en").split("-")[0]
	// use en if locale is not in languages
	if (!languages.some((l) => l.lang === locale)) {
		locale = "en"
	}
	// handle non-english locales
	if (locale !== "en") {
		dynamicActivate(locale)
	} else {
		// fallback to en
		activateLocale("en")
	}
}
