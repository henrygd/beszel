/** biome-ignore-all lint/security/noDangerouslySetInnerHtml: html comes from journalctl via agent */
import { t } from "@lingui/core/macro"
import { Trans } from "@lingui/react/macro"
import { useEffect, useRef, useState } from "react"
import { Dialog, DialogContent, DialogTitle } from "@/components/ui/dialog"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { pb } from "@/lib/api"
import { MaximizeIcon, RefreshCwIcon } from "lucide-react"
import { cn } from "@/lib/utils"

const syntaxTheme = "github-dark-dimmed"

async function getSystemLogsHtml(systemId: string, serviceName: string): Promise<string> {
	try {
		const query: Record<string, string> = { system: systemId }
		if (serviceName) {
			query.service = serviceName
		}
		const [{ highlighter }, response] = await Promise.all([
			import("@/lib/shiki"),
			pb.send<{ logs: string }>("/api/beszel/system/logs", { query }),
		])
		return response.logs
			? highlighter.codeToHtml(response.logs, { lang: "log", theme: syntaxTheme })
			: t`No results.`
	} catch (error) {
		console.error(error)
		return ""
	}
}

export default function SystemLogsTab({
	systemId,
	logsRequest,
}: {
	systemId: string
	logsRequest?: { service: string; seq: number }
}) {
	const [logsDisplay, setLogsDisplay] = useState<string>("")
	const [isRefreshing, setIsRefreshing] = useState<boolean>(false)
	const [fullscreenOpen, setFullscreenOpen] = useState<boolean>(false)
	const [serviceName, setServiceName] = useState<string>("")
	const logsContainerRef = useRef<HTMLDivElement>(null)

	function scrollToBottom() {
		if (logsContainerRef.current) {
			logsContainerRef.current.scrollTo({ top: logsContainerRef.current.scrollHeight })
		}
	}

	const fetchLogs = async (service: string) => {
		setIsRefreshing(true)
		const startTime = Date.now()
		try {
			const html = await getSystemLogsHtml(systemId, service)
			setLogsDisplay(html)
			setTimeout(scrollToBottom, 20)
		} catch (error) {
			console.error(error)
		} finally {
			const elapsed = Date.now() - startTime
			const remaining = Math.max(0, 500 - elapsed)
			setTimeout(() => setIsRefreshing(false), remaining)
		}
	}

	useEffect(() => {
		fetchLogs("")
	}, [systemId])

	useEffect(() => {
		if (logsRequest && logsRequest.seq > 0) {
			setServiceName(logsRequest.service)
			fetchLogs(logsRequest.service)
		}
	}, [logsRequest?.seq])

	return (
		<>
			<SystemLogsFullscreenDialog
				open={fullscreenOpen}
				onOpenChange={setFullscreenOpen}
				logsDisplay={logsDisplay}
				onRefresh={() => fetchLogs(serviceName)}
				isRefreshing={isRefreshing}
			/>
			<div className="flex flex-col gap-3 w-full min-w-0">
				<div className="flex items-center gap-2 ps-1">
					<Input
						placeholder={t`Filter by service (e.g. nginx.service)`}
						value={serviceName}
						onChange={(e) => setServiceName(e.target.value)}
						onKeyDown={(e) => {
							if (e.key === "Enter") fetchLogs(serviceName)
						}}
						className="min-w-0 max-w-xs h-8 text-sm"
					/>
					<Button
						variant="outline"
						size="sm"
						className="h-8"
						onClick={() => fetchLogs(serviceName)}
						disabled={isRefreshing}
					>
						<RefreshCwIcon
							className={`size-3.5 transition-transform duration-300 ${isRefreshing ? "animate-spin" : ""}`}
						/>
						<Trans>Refresh</Trans>
					</Button>
					<Button
						variant="ghost"
						size="sm"
						onClick={() => setFullscreenOpen(true)}
						className="h-8 w-8 p-0 ms-auto"
					>
						<MaximizeIcon className="size-4" />
					</Button>
				</div>
				<div
					ref={logsContainerRef}
					className={cn(
						"w-full overflow-auto p-3 rounded-md bg-gh-dark text-white text-sm max-h-[calc(100dvh-16rem)]",
						!logsDisplay && ["animate-pulse", "min-h-32"]
					)}
				>
					<div dangerouslySetInnerHTML={{ __html: logsDisplay }} />
				</div>
			</div>
		</>
	)
}

function SystemLogsFullscreenDialog({
	open,
	onOpenChange,
	logsDisplay,
	onRefresh,
	isRefreshing,
}: {
	open: boolean
	onOpenChange: (open: boolean) => void
	logsDisplay: string
	onRefresh: () => void | Promise<void>
	isRefreshing: boolean
}) {
	const outerContainerRef = useRef<HTMLDivElement>(null)

	useEffect(() => {
		if (open && logsDisplay) {
			setTimeout(() => {
				if (outerContainerRef.current) {
					outerContainerRef.current.scrollTop = outerContainerRef.current.scrollHeight
				}
			}, 50)
		}
	}, [open, logsDisplay])

	return (
		<Dialog open={open} onOpenChange={onOpenChange}>
			<DialogContent className="w-[calc(100vw-20px)] h-[calc(100dvh-20px)] max-w-none p-0 bg-gh-dark border-0 text-white">
				<DialogTitle className="sr-only">
					<Trans>System logs</Trans>
				</DialogTitle>
				<div ref={outerContainerRef} className="h-full overflow-auto">
					<div className="h-full w-full px-3 leading-relaxed rounded-md bg-gh-dark text-sm">
						<div className="py-3" dangerouslySetInnerHTML={{ __html: logsDisplay }} />
					</div>
				</div>
				<button
					onClick={onRefresh}
					className="absolute top-3 right-11 opacity-60 hover:opacity-100 p-1"
					disabled={isRefreshing}
					title={t`Refresh`}
					aria-label={t`Refresh`}
				>
					<RefreshCwIcon
						className={`size-4 transition-transform duration-300 ${isRefreshing ? "animate-spin" : ""}`}
					/>
				</button>
			</DialogContent>
		</Dialog>
	)
}
