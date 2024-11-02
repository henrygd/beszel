import { $direction } from "./stores"
import { i18n } from "@lingui/core"
import { detect, fromUrl, fromStorage, fromNavigator } from "@lingui/detect-locale"
import { messages as enMessages } from "../locales/en/messages.ts"

// const locale = detect(fromUrl("lang"), fromStorage("lang"), fromNavigator(), "en")
const locale = detect(fromStorage("lang"), fromNavigator(), "en")

// log if dev
if (import.meta.env.DEV) {
	console.log("detected locale", locale)
}

export async function dynamicActivate(locale: string) {
	try {
		const { messages } = await import(`../locales/${locale}/messages.ts`)
		i18n.load(locale, messages)
		i18n.activate(locale)
		document.documentElement.lang = locale
		$direction.set(locale.startsWith("ar") ? "rtl" : "ltr")
		localStorage.setItem("lang", locale)
	} catch (error) {
		console.error(`Error loading ${locale}`, error)
	}
}

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
} else if (locale && !locale.startsWith("en")) {
	dynamicActivate(locale.split("-")[0])
} else {
	i18n.load("en", enMessages)
	i18n.activate("en")
}
