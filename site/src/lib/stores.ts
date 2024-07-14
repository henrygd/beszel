import PocketBase from 'pocketbase'
import { atom } from 'nanostores'
import { SystemRecord } from '@/types'
import { createRouter } from '@nanostores/router'

export const pb = new PocketBase('/')

export const $router = createRouter(
	{
		home: '/',
		server: '/server/:name',
	},
	{ links: false }
)

export const navigate = (urlString: string) => {
	$router.open(urlString)
}

export const $authenticated = atom(pb.authStore.isValid)

export const $servers = atom([] as SystemRecord[])

export const $publicKey = atom('')
