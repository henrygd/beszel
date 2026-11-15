import { atom } from "nanostores"

const alertSections: Record<string, { tab: string; section: string }> = {
	CPU: { tab: "core", section: "cpu" },
	CPUIOWait: { tab: "core", section: "cpu" },
	CPUSteal: { tab: "core", section: "cpu" },
	Memory: { tab: "core", section: "memory" },
	Disk: { tab: "disk", section: "disk" },
	Bandwidth: { tab: "network", section: "bandwidth" },
	NetworkMonitorLoss: { tab: "network", section: "monitors" },
	GPU: { tab: "gpu", section: "gpu" },
	Temperature: { tab: "core", section: "temperature" },
	LoadAvg1: { tab: "core", section: "load" },
	LoadAvg5: { tab: "core", section: "load" },
	LoadAvg15: { tab: "core", section: "load" },
	Battery: { tab: "core", section: "battery" },
	ContainerHealth: { tab: "containers", section: "containers" },
	SystemdFailed: { tab: "services", section: "services" },
}

export function getAlertDestination(name: string) {
	return Object.hasOwn(alertSections, name) ? alertSections[name] : undefined
}

// Reopen the destination even when the user clicks the same alert link again.
export const $alertNavigation = atom(0)

export function scrollToAlertSection(root: HTMLElement, section: string, onComplete: () => void) {
	let frame = 0
	const scroll = () => {
		const target = root.querySelector<HTMLElement>(`[data-alert-section="${section}"]`)
		if (!target || target.getClientRects().length === 0) return
		const loaded = target.getBoundingClientRect().height > 0
		if (loaded) observer.disconnect()
		cancelAnimationFrame(frame)
		frame = requestAnimationFrame(() => {
			target.scrollIntoView({ block: "start" })
			// Scrolling an empty lazy wrapper into view starts its intersection observer.
			if (loaded) {
				target.tabIndex = -1
				target.focus({ preventScroll: true })
				onComplete()
			}
		})
	}
	const observer = new MutationObserver(scroll)
	observer.observe(root, { childList: true, subtree: true, attributes: true, attributeFilter: ["class", "hidden"] })
	scroll()
	return () => {
		observer.disconnect()
		cancelAnimationFrame(frame)
	}
}
