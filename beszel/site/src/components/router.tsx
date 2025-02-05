import { createRouter } from "@nanostores/router"

const routes = {
	home: "/",
	system: `/system/:name`,
	settings: `/settings/:name?`,
	forgot_password: `/forgot-password`,
} as const

/**
 * The base path of the application.
 * This is used to prepend the base path to all routes.
 */
export const basePath = window.BASE_PATH || ""

/**
 * Prepends the base path to the given path.
 * @param path The path to prepend the base path to.
 * @returns The path with the base path prepended.
 */
export const prependBasePath = (path: string) => (basePath + path).replaceAll("//", "/")

// prepend base path to routes
for (const route in routes) {
	// @ts-ignore need as const above to get nanostores to parse types properly
	routes[route] = prependBasePath(routes[route])
}

export const $router = createRouter(routes, { links: false })

/** Navigate to url using router
 *  Base path is automatically prepended if serving from subpath
 */
export const navigate = (urlString: string) => {
	$router.open(urlString)
}

function onClick(e: React.MouseEvent<HTMLAnchorElement, MouseEvent>) {
	e.preventDefault()
	$router.open(new URL((e.currentTarget as HTMLAnchorElement).href).pathname)
}

export const Link = (props: React.AnchorHTMLAttributes<HTMLAnchorElement>) => {
	return <a onClick={onClick} {...props}></a>
}
