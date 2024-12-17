import type { LinguiConfig } from "@lingui/conf"

const config: LinguiConfig = {
	locales: [
		"en",
		"ar",
		"cs",
		"de",
		"es",
		"fa",
		"fr",
		"hr",
		"it",
		"ja",
		"ko",
		"nl",
		"pl",
		"pt",
		"tr",
		"ru",
		"sv",
		"uk",
		"vi",
		"zh-CN",
		"zh-HK",
	],
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
