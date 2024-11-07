import type { LinguiConfig } from "@lingui/conf"

const config: LinguiConfig = {
	locales: ["en", "ar", "de", "es", "fr", "hr", "it", "ja", "ko", "pl", "pt", "tr", "ru", "uk", "vi", "zh-CN", "zh-HK"],
	sourceLocale: "en",
	compileNamespace: "ts",
	catalogs: [
		{
			path: "<rootDir>/src/locales/{locale}/{locale}",
			include: ["src"],
		},
	],
}

export default config
