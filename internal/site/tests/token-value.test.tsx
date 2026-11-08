import { describe, expect, mock, test } from "bun:test"
import { renderToStaticMarkup } from "react-dom/server"

mock.module("@lingui/core/macro", () => ({ t: (strings: TemplateStringsArray) => strings[0] }))
mock.module("../src/lib/utils", () => ({
	cn: (...values: unknown[]) => values.filter(Boolean).join(" "),
	copyToClipboard: async () => {},
}))

const { TokenValue } = await import("../src/components/token-value")
const token = "dummy-agent-token-not-a-real-credential"

describe("token display privacy", () => {
	test("does not place the token in rendered text or attributes before reveal", () => {
		const html = renderToStaticMarkup(<TokenValue token={token} label="Token for test system" />)
		expect(html).not.toContain(token)
		expect(html).toContain("Token hidden")
		expect(html).toContain('aria-label="Reveal token"')
		expect(html).toContain('aria-label="Copy token"')
		expect(html).toContain('aria-pressed="false"')
	})

	test("disabled universal tokens remain masked and their controls cannot be used", () => {
		const html = renderToStaticMarkup(<TokenValue token={token} label="Universal token" disabled />)
		expect(html).not.toContain(token)
		expect(html.match(/disabled=""/g)).toHaveLength(2)
	})
})
