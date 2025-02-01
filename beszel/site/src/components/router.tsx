import { createRouter } from "@nanostores/router"

export const baseURL = import.meta.env.BASE_URL // TODO ensure it is /.*/ ?
export const $router = createRouter(
	{
		home: baseURL,
		server: baseURL + "system/:name",
		settings: baseURL + "settings/:name?",
		forgot_password: baseURL + "forgot-password",
	},
	{ links: false }
)

/** Navigate to url using router */
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
