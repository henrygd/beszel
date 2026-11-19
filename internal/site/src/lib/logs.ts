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
