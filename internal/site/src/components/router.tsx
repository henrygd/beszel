import { createRouter, getPagePath } from "@nanostores/router"

const routes = {
	home: "/",
	containers: "/containers",
	smart: "/smart",
	monitors: "/monitors",
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

/** Pages a user can choose to open by default (Settings > General) */
export const defaultPages = ["home", "containers", "smart", "monitors"] as const
export type DefaultPage = (typeof defaultPages)[number]

/** Path of the given default page, or the systems page if it isn't set or isn't a valid choice */
export const getDefaultPagePath = (page?: string) =>
	getPagePath($router, defaultPages.includes(page as DefaultPage) ? (page as DefaultPage) : "home")

/**
 * Opens the user's default page in place of the systems page.
 * Does nothing if the user has navigated away from startPath in the meantime.
 */
export const openDefaultPage = (page: string | undefined, startPath: string | undefined) => {
	const current = $router.get()
	if (!page || page === "home" || current?.route !== "home" || current.path !== startPath) {
		return
	}
	$router.open(getDefaultPagePath(page), true)
}

/** Navigate to url using router
 *  Base path is automatically prepended if serving from subpath
 */
export const navigate = (urlString: string) => {
	$router.open(urlString)
}

export function Link({ href, ...props }: React.AnchorHTMLAttributes<HTMLAnchorElement> & { href: string }) {
	return (
		<a
			{...props}
			href={href}
			onClick={(e) => {
				e.preventDefault()
				if (e.ctrlKey || e.metaKey) {
					window.open(href, "_blank")
				} else {
					navigate(href)
					props.onClick?.(e)
				}
			}}
		></a>
	)
}
