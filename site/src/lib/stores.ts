import PocketBase from 'pocketbase'
import { atom } from 'nanostores'
import { SystemRecord } from '@/types'

export const pb = new PocketBase('/')
// @ts-ignore
pb.authStore.storageKey = 'pb_admin_auth'

export const $servers = atom([] as SystemRecord[])

export const $authenticated = atom(pb.authStore.isValid)
pb.authStore.onChange(() => {
	$authenticated.set(pb.authStore.isValid)
})

export const $publicKey = atom('')
