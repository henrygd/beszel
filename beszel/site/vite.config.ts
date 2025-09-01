import { defineConfig } from "vite"
import path from "path"
import tailwindcss from "@tailwindcss/vite"
import react from "@vitejs/plugin-react-swc"
import { lingui } from "@lingui/vite-plugin"
import { version } from "./package.json"

export default defineConfig({
	base: "./",
	plugins: [
		react({
			plugins: [["@lingui/swc-plugin", {}]],
		}),
		lingui(),
		tailwindcss(),
		{
			name: "replace version in index.html during dev",
			apply: "serve",
			transformIndexHtml(html) {
				return html.replace("{{V}}", version).replace("{{HUB_URL}}", "")
			},
		},
	],
	esbuild: {
		legalComments: "external",
	},
	resolve: {
		alias: {
			"@": path.resolve(__dirname, "./src"),
		},
	},
})
