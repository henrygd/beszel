/** Parses a filter string into OR'd groups of AND'd terms: "a b, c" -> [["a","b"], ["c"]] */
export function parseFilterGroups(value: string): string[][] {
	return value
		.toLowerCase()
		.split(",")
		.map((group) => group.trim().split(" ").filter((term) => term.length > 0))
		.filter((terms) => terms.length > 0)
}

/** True if every term in at least one OR'd group is found in searchString. */
export function matchesFilterGroups(searchString: string, groups: string[][]): boolean {
	return groups.some((terms) => terms.every((term) => searchString.includes(term)))
}

// Table filter functions run once per row with the same filter value, so cache the last parse.
let lastFilterValue = ""
let lastFilterGroups: string[][] = []

/**
 * Shared table filter: matches `filterValue` ("a b, c" = (a AND b) OR c) against the given fields,
 * case-insensitively. Fields are joined with spaces so a term can't match across two fields.
 * An empty filter matches everything.
 */
export function matchesTableFilter(filterValue: string, ...fields: (string | number | null | undefined)[]): boolean {
	if (filterValue !== lastFilterValue) {
		lastFilterValue = filterValue
		lastFilterGroups = parseFilterGroups(filterValue)
	}
	if (!lastFilterGroups.length) {
		return true
	}
	return matchesFilterGroups(fields.filter((field) => field != null).join(" ").toLowerCase(), lastFilterGroups)
}
