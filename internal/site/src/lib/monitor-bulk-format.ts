/** Read one bulk-monitor row, including CSV-quoted targets containing commas. */
export function parseMonitorFields(line: string): string[] {
	const fields: string[] = []
	let position = 0
	while (position <= line.length) {
		let value = ""
		if (line[position] === '"') {
			position++
			let closed = false
			while (position < line.length) {
				const character = line[position++]
				if (character !== '"') {
					value += character
				} else if (line[position] === '"') {
					value += '"'
					position++
				} else {
					closed = true
					break
				}
			}
			if (!closed) throw new Error("Unclosed quoted field")
			while (position < line.length && /\s/.test(line[position])) position++
			if (position < line.length && line[position] !== ",") throw new Error("Expected comma after quoted field")
		} else {
			const end = line.indexOf(",", position)
			const next = end === -1 ? line.length : end
			value = line.slice(position, next)
			position = next
		}
		fields.push(value)
		if (position === line.length) break
		position++
	}
	return fields
}

export function formatMonitorFields(fields: string[]): string {
	return fields.map((field) => (/[",]/.test(field) ? `"${field.replaceAll('"', '""')}"` : field)).join(",")
}
