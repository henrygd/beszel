import { describe, expect, test } from "bun:test"
import { getLogDecorations, getLogLinkDecorations } from "../src/lib/logs"

const { highlighter } = await import("../src/lib/shiki")

const linkedUrls = (logs: string) =>
	getLogLinkDecorations(logs).map(({ start, end }) => logs.slice(start as number, end as number))

const render = (logs: string) =>
	highlighter.codeToHtml(logs, { lang: "log", theme: "github-dark-dimmed", decorations: getLogDecorations(logs) })

// Text inside each rendered <a>, with the highlighted token spans removed
const linkTexts = (html: string) =>
	Array.from(html.matchAll(/<a [^>]*>(.*?)<\/a>/g), (match) => match[1].replace(/<[^>]+>/g, ""))

describe("log link decorations", () => {
	test("finds http and https URLs", () => {
		expect(linkedUrls("started at http://localhost:8080/ and HTTPS://Example.com/a?b=1&c=2#d")).toEqual([
			"http://localhost:8080/",
			"HTTPS://Example.com/a?b=1&c=2#d",
		])
	})

	test("leaves out surrounding punctuation, quotes and brackets", () => {
		expect(linkedUrls("see https://example.com/docs.")).toEqual(["https://example.com/docs"])
		expect(linkedUrls('msg="open https://example.com/x"')).toEqual(["https://example.com/x"])
		expect(linkedUrls("(https://example.com/a), [https://example.com/b]")).toEqual([
			"https://example.com/a",
			"https://example.com/b",
		])
		expect(linkedUrls("<https://example.com/c>")).toEqual(["https://example.com/c"])
	})

	test("keeps brackets that are part of the URL", () => {
		expect(linkedUrls("see (https://en.wikipedia.org/wiki/Go_(game)).")).toEqual([
			"https://en.wikipedia.org/wiki/Go_(game)",
		])
		expect(linkedUrls("metrics at http://[::1]:9000/metrics")).toEqual(["http://[::1]:9000/metrics"])
	})

	test("ignores other schemes and incomplete URLs", () => {
		expect(linkedUrls("javascript:alert(1) data:text/html,x ftp://example.com xhttp://example.com https://.")).toEqual(
			[]
		)
	})
})

describe("highlighted log links", () => {
	test("renders links that open in a new tab", () => {
		const html = render("2026-11-28T10:00:00Z Web vault at http://localhost:8000/ ready")
		expect(html).toContain(
			'<a href="http://localhost:8000/" target="_blank" rel="noopener noreferrer" class="log-link">'
		)
		expect(html).toContain('class="log-timestamp"')
		expect(linkTexts(html)).toEqual(["http://localhost:8000/"])
	})

	test("wraps a URL split across highlighted tokens in one link", () => {
		const url = "https://en.wikipedia.org/wiki/Go_(game)"
		const tokens = highlighter.codeToTokensBase(`see (${url}).`, { lang: "log", theme: "github-dark-dimmed" })[0]
		// The log grammar ends its URL token before ")", so the URL spans more than one token.
		expect(tokens.some((token) => token.content === url)).toBe(false)
		expect(linkTexts(render(`see (${url}).`))).toEqual([url])
	})

	test("escapes the href and never turns log text into markup", () => {
		const html = render('a=https://example.com/?a=1&b=2 b=https://example.com/"><img src=x onerror=alert(1)>')
		expect(html).toContain('href="https://example.com/?a=1&#x26;b=2"')
		expect(html).toContain('href="https://example.com/"')
		expect(html).not.toContain("<img")
		expect(html).toContain("&#x3C;img")
	})
})
