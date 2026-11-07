import { afterEach, beforeEach, describe, expect, mock, test } from "bun:test"
import { getPagePath } from "@nanostores/router"
import { $router } from "../src/components/router"
import { getAlertDestination, scrollToAlertSection } from "../src/lib/alert-navigation"

describe("alert destinations", () => {
	test.each([
		["CPU", "core", "cpu"],
		["CPUIOWait", "core", "cpu"],
		["CPUSteal", "core", "cpu"],
		["Memory", "core", "memory"],
		["Temperature", "core", "temperature"],
		["LoadAvg1", "core", "load"],
		["LoadAvg5", "core", "load"],
		["LoadAvg15", "core", "load"],
		["Battery", "core", "battery"],
		["Disk", "disk", "disk"],
		["Bandwidth", "network", "bandwidth"],
		["NetworkMonitorLoss", "network", "monitors"],
		["GPU", "gpu", "gpu"],
		["ContainerHealth", "containers", "containers"],
		["SystemdFailed", "services", "services"],
	])("routes %s alerts to the %s tab and %s section", (name, tab, section) => {
		expect(getAlertDestination(name)).toEqual({ tab, section })
	})

	test.each(["Status", "UnknownAlert", "constructor", "__proto__"])("keeps %s on the system overview", (name) => {
		expect(getAlertDestination(name)).toBeUndefined()
	})
})

describe("scrolling to asynchronously rendered sections", () => {
	const originalObserver = globalThis.MutationObserver
	const originalRequestFrame = globalThis.requestAnimationFrame
	const originalCancelFrame = globalThis.cancelAnimationFrame
	let mutate: () => void
	let height: number
	let visible: boolean
	let present: boolean
	let root: HTMLElement
	const frames = new Map<number, FrameRequestCallback>()
	const scroll = mock(() => {})
	const disconnect = mock(() => {})
	const complete = mock(() => {})

	function flushFrames() {
		for (const [id, callback] of frames) {
			frames.delete(id)
			callback(0)
		}
	}

	beforeEach(() => {
		height = 200
		visible = true
		present = true
		scroll.mockClear()
		disconnect.mockClear()
		complete.mockClear()
		frames.clear()
		let nextFrame = 0
		globalThis.requestAnimationFrame = (callback) => {
			frames.set(++nextFrame, callback)
			return nextFrame
		}
		globalThis.cancelAnimationFrame = (id) => {
			frames.delete(id)
		}
		globalThis.MutationObserver = class {
			constructor(callback: MutationCallback) {
				mutate = () => callback([], this as unknown as MutationObserver)
			}
			observe() {}
			disconnect = disconnect
		} as unknown as typeof MutationObserver
		const target = {
			getClientRects: () => (visible ? [{}] : []),
			getBoundingClientRect: () => ({ height }),
			scrollIntoView: scroll,
			focus: () => {},
		}
		root = { querySelector: () => (present ? target : null) } as unknown as HTMLElement
	})

	afterEach(() => {
		globalThis.MutationObserver = originalObserver
		globalThis.requestAnimationFrame = originalRequestFrame
		globalThis.cancelAnimationFrame = originalCancelFrame
	})

	test("scrolls a lazy placeholder into view before waiting for its table to load", () => {
		height = 0
		const cleanup = scrollToAlertSection(root, "services", complete)
		flushFrames()
		expect(scroll).toHaveBeenCalledTimes(1)
		expect(complete).not.toHaveBeenCalled()
		expect(disconnect).not.toHaveBeenCalled()
		height = 200
		mutate()
		flushFrames()
		expect(scroll).toHaveBeenCalledTimes(2)
		expect(complete).toHaveBeenCalledTimes(1)
		expect(disconnect).toHaveBeenCalledTimes(1)
		cleanup()
	})

	test("waits for a hidden tab to become visible before scrolling", () => {
		visible = false
		const cleanup = scrollToAlertSection(root, "services", complete)
		flushFrames()
		expect(scroll).not.toHaveBeenCalled()
		visible = true
		mutate()
		flushFrames()
		expect(scroll).toHaveBeenCalledWith({ block: "start" })
		expect(complete).toHaveBeenCalledTimes(1)
		cleanup()
	})

	test("waits for a data-dependent chart to be inserted", () => {
		present = false
		const cleanup = scrollToAlertSection(root, "temperature", complete)
		flushFrames()
		expect(scroll).not.toHaveBeenCalled()
		present = true
		mutate()
		flushFrames()
		expect(scroll).toHaveBeenCalledTimes(1)
		cleanup()
	})

	test("cancels pending scrolling when navigating away", () => {
		const cleanup = scrollToAlertSection(root, "services", complete)
		cleanup()
		flushFrames()
		expect(scroll).not.toHaveBeenCalled()
		expect(complete).not.toHaveBeenCalled()
		expect(disconnect).toHaveBeenCalled()
	})
})

describe("system alert routes", () => {
	test("preserves existing system URLs and legacy system-name slugs", () => {
		const path = getPagePath($router, "system", { id: "Web Server" })
		expect(path).toBe("/system/Web%20Server")
		$router.open(path)
		expect($router.get()?.params).toEqual({ id: "Web Server", alert: "" })
	})

	test("preserves an alert destination in a directly opened URL", () => {
		const path = getPagePath($router, "system", { id: "test-system", alert: "SystemdFailed" })
		expect(path).toBe("/system/test-system/SystemdFailed")
		$router.open(path)
		expect($router.get()?.params).toEqual({ id: "test-system", alert: "SystemdFailed" })
	})

	test("notifies subscribers when navigating between alerts on the same system", () => {
		const changes: string[] = []
		const unsubscribe = $router.listen((page) => changes.push(page?.path ?? ""))
		$router.open("/system/same-system/Memory")
		$router.open("/system/same-system/SystemdFailed")
		unsubscribe()
		expect(changes).toEqual(["/system/same-system/Memory", "/system/same-system/SystemdFailed"])
	})
})
