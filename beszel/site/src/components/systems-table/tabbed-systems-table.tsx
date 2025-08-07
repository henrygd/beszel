import { Suspense, lazy, memo, useEffect } from "react"
import { Card, CardContent, CardHeader, CardTitle } from "../ui/card"
import { $tailscaleEnabled, $tailscaleNodes, pb } from "@/lib/stores"
import { useStore } from "@nanostores/react"
import { useLingui, Trans } from "@lingui/react/macro"
import { ServerIcon } from "lucide-react"
import { TailscaleIcon } from "../ui/icons"
import { Tabs, TabsContent, TabsList, TabsTrigger } from "../ui/tabs"

const SystemsTable = lazy(() => import("./systems-table"))
const TailscaleTable = lazy(() => import("../tailscale-table/tailscale-table"))

export default function TabbedSystemsTable() {
	const tailscaleEnabled = useStore($tailscaleEnabled)

	// Check if Tailscale is enabled by trying to fetch the API
	useEffect(() => {
		const checkTailscaleEnabled = async () => {
			try {
				// Use the current window location to construct the API URL
				const apiUrl = `${window.location.origin}/api/beszel/tailscale/stats`
				
				const response = await fetch(apiUrl, {
					headers: {
						Authorization: pb.authStore.token,
					},
				})
				// Consider both 200 (success) and 403 (forbidden but endpoint exists) as enabled
				const isEnabled = response.status === 200 || response.status === 403
				$tailscaleEnabled.set(isEnabled)
			} catch (error) {
				$tailscaleEnabled.set(false)
			}
		}

		checkTailscaleEnabled()
	}, [])

	// If Tailscale is not enabled, only show systems tab
	if (!tailscaleEnabled) {
		return (
			<Suspense>
				<SystemsTable />
			</Suspense>
		)
	}

	return (
		<Tabs defaultValue="systems" className="w-full">
			<TabsList className="grid w-full grid-cols-2">
				<TabsTrigger value="systems" className="flex items-center gap-2">
					<ServerIcon className="h-4 w-4" />
					<Trans>Systems</Trans>
				</TabsTrigger>
				<TabsTrigger value="tailscale" className="flex items-center gap-2">
					<TailscaleIcon className="h-4 w-4" />
					<Trans>Tailscale</Trans>
				</TabsTrigger>
			</TabsList>
			<TabsContent value="systems" className="mt-6">
				<Suspense>
					<SystemsTable />
				</Suspense>
			</TabsContent>
			<TabsContent value="tailscale" className="mt-6">
				<Suspense>
					<TailscaleTable />
				</Suspense>
			</TabsContent>
		</Tabs>
	)
} 