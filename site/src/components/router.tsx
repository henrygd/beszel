import { createRouter } from '@nanostores/router'

export const $router = createRouter(
	{
		home: '/',
		server: '/server/:name',
		'forgot-password': '/forgot-password',
	},
	{ links: false }
)

/** Navigate to url using router */
export const navigate = (urlString: string) => {
	$router.open(urlString)
}

function onClick(e: React.MouseEvent<HTMLAnchorElement, MouseEvent>) {
	e.preventDefault()
	$router.open(new URL((e.target as HTMLAnchorElement).href).pathname)
}

export const Link = (props: React.AnchorHTMLAttributes<HTMLAnchorElement>) => {
	return <a onClick={onClick} {...props}></a>
}
