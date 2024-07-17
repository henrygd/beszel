import PocketBase from 'pocketbase'
import { atom, WritableAtom } from 'nanostores'
import { AlertRecord, ChartTimes, SystemRecord } from '@/types'

/** PocketBase JS Client */
export const pb = new PocketBase('/')

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
export const $chartTime = atom('1h') as WritableAtom<ChartTimes>
