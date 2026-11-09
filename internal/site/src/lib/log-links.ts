import type { Element, ElementContent } from "hast"
import type { ShikiTransformer } from "shiki"

/** Only explicit HTTP(S) URLs in log text are eligible for navigation. */
export function findLogUrls(text: string) {
	const matches: { start: number; end: number; url: string }[] = []
	// biome-ignore lint/suspicious/noControlCharactersInRegex: control bytes must terminate untrusted log URLs
	for (const match of text.matchAll(/\bhttps?:\/\/[^\s<>"'`\x00-\x1f]+/gi)) {
		let url = match[0].replace(/[.,;:!?]+$/, "")
		// Keep balanced URL parentheses (e.g. Wikipedia paths), but drop prose delimiters.
		for (const [open, close] of [
			["(", ")"],
			["[", "]"],
			["{", "}"],
		]) {
			while (url.endsWith(close) && url.split(close).length > url.split(open).length) {
				url = url.slice(0, -1)
			}
		}
		try {
			const parsed = new URL(url)
			if (parsed.protocol !== "http:" && parsed.protocol !== "https:") continue
			matches.push({ start: match.index, end: match.index + url.length, url })
		} catch {
			// A partial URL in a truncated log line is still ordinary text.
		}
	}
	return matches
}

function textContent(node: ElementContent): string {
	if (node.type === "text") return node.value
	return node.type === "element" ? node.children.map(textContent).join("") : ""
}

// Slice the highlighted tree by text offsets, preserving spans and timestamp decorations.
function sliceChildren(children: ElementContent[], start: number, end: number): ElementContent[] {
	const result: ElementContent[] = []
	let offset = 0
	for (const child of children) {
		const length = textContent(child).length
		if (offset < end && offset + length > start) {
			const from = Math.max(0, start - offset)
			const to = Math.min(length, end - offset)
			if (child.type === "text") result.push({ ...child, value: child.value.slice(from, to) })
			else if (child.type === "element") {
				result.push({ ...child, children: sliceChildren(child.children, from, to) })
			}
		}
		offset += length
	}
	return result
}

export function linkifyLogLine(line: Element) {
	const text = line.children.map(textContent).join("")
	const matches = findLogUrls(text)
	if (!matches.length) return
	const children: ElementContent[] = []
	let offset = 0
	for (const { start, end, url } of matches) {
		children.push(...sliceChildren(line.children, offset, start))
		children.push({
			type: "element",
			tagName: "a",
			properties: {
				href: url,
				"data-log-url": url,
				target: "_blank",
				rel: "noopener noreferrer",
				className: ["underline", "underline-offset-2", "hover:opacity-80", "focus-visible:outline"],
			},
			children: sliceChildren(line.children, start, end),
		})
		offset = end
	}
	children.push(...sliceChildren(line.children, offset, text.length))
	line.children = children
}

export const logLinksTransformer: ShikiTransformer = {
	name: "beszel-log-links",
	line: linkifyLogLine,
}
