import { atom } from "nanostores"

// One polite announcement for a page update, not one for each metric card.
export const $tableUpdates = atom<ReadonlySet<string>>(new Set())
export function markTableUpdate(id: string, pending: boolean) {
	const current = $tableUpdates.get()
	if (current.has(id) === pending) return
	const next = new Set(current)
	if (pending) next.add(id)
	else next.delete(id)
	$tableUpdates.set(next)
}
