import { useEffect, useRef, useState } from "react"

// adapted from usehooks-ts/use-intersection-observer

/** The hook internal state. */
type State = {
	/** A boolean indicating if the element is intersecting. */
	isIntersecting: boolean
	/** The intersection observer entry. */
	entry?: IntersectionObserverEntry
}

/** Represents the options for configuring the Intersection Observer. */
type UseIntersectionObserverOptions = {
	/**
	 * The element that is used as the viewport for checking visibility of the target.
	 * @default null
	 */
	root?: Element | Document | null
	/**
	 * A margin around the root.
	 * @default '0%'
	 */
	rootMargin?: string
	/**
	 * A threshold indicating the percentage of the target's visibility needed to trigger the callback.
	 * @default 0
	 */
	threshold?: number | number[]
	/**
	 * If true, freezes the intersection state once the element becomes visible.
	 * @default true
	 */
	freeze?: boolean
	/**
	 * A callback function to be invoked when the intersection state changes.
	 * @param {boolean} isIntersecting - A boolean indicating if the element is intersecting.
	 * @param {IntersectionObserverEntry} entry - The intersection observer Entry.
	 * @default undefined
	 */
	onChange?: (isIntersecting: boolean, entry: IntersectionObserverEntry) => void
	/**
	 * The initial state of the intersection.
	 * @default false
	 */
	initialIsIntersecting?: boolean
}

/**
 * The return type of the useIntersectionObserver hook.
 *
 * Supports both tuple and object destructing.
 * @param {(node: Element | null) => void} ref - The ref callback function.
 * @param {boolean} isIntersecting - A boolean indicating if the element is intersecting.
 * @param {IntersectionObserverEntry | undefined} entry - The intersection observer Entry.
 */
type IntersectionReturn = {
	ref: (node?: Element | null) => void
	isIntersecting: boolean
	entry?: IntersectionObserverEntry
}

/**
 * Custom hook that tracks the intersection of a DOM element with its containing element or the viewport using the [`Intersection Observer API`](https://developer.mozilla.org/en-US/docs/Web/API/Intersection_Observer_API).
 * @param {UseIntersectionObserverOptions} options - The options for the Intersection Observer.
 * @returns {IntersectionReturn} The ref callback, a boolean indicating if the element is intersecting, and the intersection observer entry.
 * @example
 * ```tsx
 * const { ref, isIntersecting, entry } = useIntersectionObserver({ threshold: 0.5 });
 * ```
 */
export function useIntersectionObserver({
	threshold = 0,
	root = null,
	rootMargin = "0%",
	freeze = true,
	initialIsIntersecting = false,
	onChange,
}: UseIntersectionObserverOptions = {}): IntersectionReturn {
	const [ref, setRef] = useState<Element | null>(null)

	const [state, setState] = useState<State>(() => ({
		isIntersecting: initialIsIntersecting,
		entry: undefined,
	}))

	const callbackRef = useRef<UseIntersectionObserverOptions["onChange"]>()

	callbackRef.current = onChange

	const frozen = state.entry?.isIntersecting && freeze

	useEffect(() => {
		// Ensure we have a ref to observe
		if (!ref) return

		// Ensure the browser supports the Intersection Observer API
		if (!("IntersectionObserver" in window)) return

		// Skip if frozen
		if (frozen) return

		let unobserve: (() => void) | undefined

		const observer = new IntersectionObserver(
			(entries: IntersectionObserverEntry[]): void => {
				const thresholds = Array.isArray(observer.thresholds) ? observer.thresholds : [observer.thresholds]

				entries.forEach((entry) => {
					const isIntersecting =
						entry.isIntersecting && thresholds.some((threshold) => entry.intersectionRatio >= threshold)

					setState({ isIntersecting, entry })

					if (callbackRef.current) {
						callbackRef.current(isIntersecting, entry)
					}

					if (isIntersecting && freeze && unobserve) {
						unobserve()
						unobserve = undefined
					}
				})
			},
			{ threshold, root, rootMargin }
		)

		observer.observe(ref)

		return () => {
			observer.disconnect()
		}

		// eslint-disable-next-line react-hooks/exhaustive-deps
	}, [
		ref,
		// eslint-disable-next-line react-hooks/exhaustive-deps
		JSON.stringify(threshold),
		root,
		rootMargin,
		frozen,
		freeze,
	])

	// ensures that if the observed element changes, the intersection observer is reinitialized
	const prevRef = useRef<Element | null>(null)

	useEffect(() => {
		if (!ref && state.entry?.target && !freeze && !frozen && prevRef.current !== state.entry.target) {
			prevRef.current = state.entry.target
			setState({ isIntersecting: initialIsIntersecting, entry: undefined })
		}
	}, [ref, state.entry, freeze, frozen, initialIsIntersecting])

	return {
		ref: setRef,
		isIntersecting: !!state.isIntersecting,
		entry: state.entry,
	} as IntersectionReturn
}
