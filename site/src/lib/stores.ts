import PocketBase from 'pocketbase'
import { atom } from 'nanostores'
import { AlertRecord, SystemRecord } from '@/types'
import { createRouter } from '@nanostores/router'

/** PocketBase JS Client */
export const pb = new PocketBase('/')

export const $router = createRouter(
	{
		home: '/',
		server: '/server/:name',
	},
	{ links: false }
)

/** Navigate to url using router */
export const navigate = (urlString: string) => {
	$router.open(urlString)
}

/** Store if user is authenticated */
export const $authenticated = atom(pb.authStore.isValid)

/** List of system records */
export const $systems = atom([] as SystemRecord[])

/** Last updated system record (realtime) */
export const $updatedSystem = atom({} as SystemRecord)

/** List of alert records */
export const $alerts = atom([] as AlertRecord[])

/** SSH public key */
export const $publicKey = atom('')

/** Chart time period */
export const $chartTime = atom('1h')
