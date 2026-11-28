import type { DecorationItem } from "shiki/core"

// Docker uses RFC3339Nano; journalctl short-iso uses an offset such as +0200.
// Only mark the first timestamp, leaving dates in the log message untouched.
export function getLogTimestampDecorations(logs: string): DecorationItem[] {
	const timestamps = logs.matchAll(/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d+)?(?:Z|[+-]\d{2}:?\d{2}) /gm)
	return Array.from(timestamps, (match) => ({
		start: match.index,
		end: match.index + match[0].length,
		properties: { class: "log-timestamp" },
		alwaysWrap: true,
	}))
}

// Quotes, angle brackets and whitespace usually delimit a URL in log output.
const urlPattern = /\bhttps?:\/\/[^\s"'`<>]+/gi
const openingBrackets: Record<string, string> = { ")": "(", "]": "[", "}": "{" }

// Drop trailing punctuation and closing brackets that belong to the surrounding text,
// e.g. "(see https://example.com/docs)." while keeping https://example.com/wiki/A_(b).
function trimUrl(url: string): string {
	while (url) {
		const last = url[url.length - 1]
		const open = openingBrackets[last]
		const trailing = open ? url.split(last).length > url.split(open).length : ".,;:!?".includes(last)
		if (!trailing) {
			break
		}
		url = url.slice(0, -1)
	}
	return url
}

// Wraps http(s) URLs in links. Shiki splits tokens at decoration boundaries, so URLs spanning
// several highlighted tokens become a single link, and the href is escaped when the HAST is serialized.
export function getLogLinkDecorations(logs: string): DecorationItem[] {
	const decorations: DecorationItem[] = []
	for (const match of logs.matchAll(urlPattern)) {
		const url = trimUrl(match[0])
		if (!/^https?:\/\/./i.test(url)) {
			continue
		}
		decorations.push({
			start: match.index,
			end: match.index + url.length,
			tagName: "a",
			properties: { href: url, target: "_blank", rel: "noopener noreferrer", class: "log-link" },
			alwaysWrap: true,
		})
	}
	return decorations
}

export function getLogDecorations(logs: string): DecorationItem[] {
	return [...getLogTimestampDecorations(logs), ...getLogLinkDecorations(logs)]
}
