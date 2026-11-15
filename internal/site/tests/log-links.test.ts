import { describe, expect, test } from "bun:test"
import type { Element } from "hast"
import { findLogUrls, linkifyLogLine, logLinksTransformer } from "../src/lib/log-links"
import { getLogTimestampDecorations } from "../src/lib/logs"
import { highlighter } from "../src/lib/shiki"

describe("URLs in log output", () => {
	test("keeps queries, fragments and balanced parentheses, dropping prose punctuation", () => {
		const text = "See (https://example.com/a_(b)?x=1&y=2#part). http://localhost:3000/path,"
		expect(findLogUrls(text).map(({ url }) => url)).toEqual([
			"https://example.com/a_(b)?x=1&y=2#part",
			"http://localhost:3000/path",
		])
		for (const { start, end, url } of findLogUrls(text)) expect(text.slice(start, end)).toBe(url)
	})

	test("excludes other schemes, bare domains, malformed URLs and control bytes", () => {
		expect(findLogUrls("javascript:alert(1) data:text/html,test ftp://example.com example.com https://")).toEqual([])
		expect(findLogUrls("https://example.com/a\u0000hidden")[0].url).toBe("https://example.com/a")
	})

	test("wraps a URL crossing syntax tokens without changing styles or timestamp nodes", () => {
		const line: Element = {
			type: "element",
			tagName: "span",
			properties: { className: ["line"] },
			children: [
				{
					type: "element",
					tagName: "span",
					properties: { className: ["log-timestamp"] },
					children: [{ type: "text", value: "12:00 " }],
				},
				{
					type: "element",
					tagName: "span",
					properties: { style: "color:red" },
					children: [{ type: "text", value: "https://example.com/" }],
				},
				{
					type: "element",
					tagName: "span",
					properties: { style: "color:blue" },
					children: [{ type: "text", value: "path done" }],
				},
			],
		}
		linkifyLogLine(line)
		expect(line.children[0]).toMatchObject({ properties: { className: ["log-timestamp"] } })
		expect(line.children[1]).toMatchObject({
			tagName: "a",
			properties: { href: "https://example.com/path", rel: "noopener noreferrer", target: "_blank" },
			children: [
				{ properties: { style: "color:red" }, children: [{ value: "https://example.com/" }] },
				{ properties: { style: "color:blue" }, children: [{ value: "path" }] },
			],
		})
		expect(line.children[2]).toMatchObject({ children: [{ value: " done" }] })
	})

	test("real Shiki output escapes HTML, preserves decorations and emits multiple links", () => {
		const logs = "2026-11-09T12:00:00Z <script>alert(1)</script> https://example.com/?a=1&b=2 http://[::1]:8080/test"
		const html = highlighter.codeToHtml(logs, {
			lang: "log",
			theme: "github-dark-dimmed",
			decorations: getLogTimestampDecorations(logs),
			transformers: [logLinksTransformer],
		})
		expect(html).not.toContain("<script>")
		expect(html).toContain("&#x3C;script>")
		expect(html).toContain('class="log-timestamp"')
		expect(html.match(/<a /g)).toHaveLength(2)
		expect(html).toContain('href="https://example.com/?a=1&#x26;b=2"')
		expect(html).toContain('href="http://[::1]:8080/test"')
	})
})
