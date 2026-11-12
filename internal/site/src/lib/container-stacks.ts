/**
 * Whether a container chart series belongs to a stack. Containers that no longer exist are
 * missing from `stacks`, so they fall back to the names Swarm and Compose give them.
 */
export function containerInStack(name: string, stack: string, stacks: Record<string, string>) {
	if (name in stacks) {
		return stacks[name] === stack
	}
	return name.startsWith(`${stack}_`) || name.startsWith(`${stack}-`)
}
