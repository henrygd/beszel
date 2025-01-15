import type { LinguiConfig } from "@lingui/conf"

const config: LinguiConfig = {
	locales: [
		"en",
		"ar",
		"bg",
		"cs",
		"da",
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
		"sl",
		"sv",
		"uk",
		"vi",
		"zh",
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
