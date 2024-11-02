import type { LinguiConfig } from "@lingui/conf"

const config: LinguiConfig = {
	locales: ["en", "ar", "de", "es", "fr", "it", "ja", "ko", "pt", "tr", "ru", "uk", "vi", "zh-CN", "zh-HK"],
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
