/** biome-ignore-all lint/a11y: anchors retain native behavior and use the client router only for unmodified primary clicks */
import { createRouter } from "@nanostores/router"

const routes = {
	home: "/",
	containers: "/containers",
	smart: "/smart",
	system: `/system/:id`,
	settings: `/settings/:name?`,
	forgot_password: `/forgot-password`,
	request_otp: `/request-otp`,
} as const

/**
 * The base path of the application.
 * This is used to prepend the base path to all routes.
 */
export const basePath = globalThis.BESZEL?.BASE_PATH || ""

/**
 * Prepends the base path to the given path.
 * @param path The path to prepend the base path to.
 * @returns The path with the base path prepended.
 */
export const prependBasePath = (path: string) => (basePath + path).replaceAll("//", "/")

// prepend base path to routes
for (const route in routes) {
	// @ts-expect-error need as const above to get nanostores to parse types properly
	routes[route] = prependBasePath(routes[route])
}

export const $router = createRouter(routes, { links: false })

/** Navigate to url using router
 *  Base path is automatically prepended if serving from subpath
 */
export const navigate = (urlString: string) => {
	$router.open(urlString)
}

export function Link(props: React.AnchorHTMLAttributes<HTMLAnchorElement>) {
	return (
		<a
			{...props}
			onClick={(e) => {
				props.onClick?.(e)
				if (
					e.defaultPrevented ||
					e.button !== 0 ||
					e.ctrlKey ||
					e.metaKey ||
					e.shiftKey ||
					e.altKey ||
					props.target === "_blank" ||
					props.download ||
					!props.href
				) {
					return
				}
				e.preventDefault()
				navigate(props.href)
			}}
		></a>
	)
}
