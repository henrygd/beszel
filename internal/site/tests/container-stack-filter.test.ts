import { describe, expect, test } from "bun:test"
import { containerInStack } from "../src/lib/container-stacks"

describe("containerInStack", () => {
	const stacks = { "immich-server": "immich", "immich-backup": "backup", "web.1.abc": "site" }

	test("uses the stack of current containers", () => {
		expect(containerInStack("immich-server", "immich", stacks)).toBe(true)
		// named like the stack, but the label says otherwise
		expect(containerInStack("immich-backup", "immich", stacks)).toBe(false)
		expect(containerInStack("web.1.abc", "site", stacks)).toBe(true)
	})

	test("falls back to Swarm and Compose names for containers that are gone", () => {
		expect(containerInStack("immich_server.1.oldtask", "immich", stacks)).toBe(true)
		expect(containerInStack("immich-redis-1", "immich", stacks)).toBe(true)
		expect(containerInStack("immichapp", "immich", stacks)).toBe(false)
		expect(containerInStack("postgres", "immich", stacks)).toBe(false)
	})
})
