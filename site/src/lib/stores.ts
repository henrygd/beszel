import PocketBase from 'pocketbase'
import { atom } from 'nanostores'
import { SystemRecord } from '@/types'
import { createRouter } from '@nanostores/router'

export const pb = new PocketBase('/')
// @ts-ignore
pb.authStore.storageKey = 'pb_admin_auth'

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
pb.authStore.onChange(() => {
	$authenticated.set(pb.authStore.isValid)
})

export const $servers = atom([] as SystemRecord[])

export const $publicKey = atom('')
