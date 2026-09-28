import { useCallback, useEffect, useRef, useState } from "react"
import { Trans, useLingui } from "@lingui/react/macro"
import { CheckIcon, ChevronDownIcon, GlobeIcon, LoaderCircleIcon, SearchIcon } from "lucide-react"
import { pb } from "@/lib/api"
import { cn } from "@/lib/utils"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import {
	DropdownMenu,
	DropdownMenuCheckboxItem,
	DropdownMenuContent,
	DropdownMenuItem,
	DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"

/** An Ookla server as returned by the hub. Zero ID means automatic server selection. */
export interface SpeedtestServer {
	id: number
	name: string
	location: string
}

export const AUTOMATIC_SERVER: SpeedtestServer = { id: 0, name: "", location: "" }

const SEARCH_DEBOUNCE_MS = 300

/**
 * Searchable select for Ookla speedtest servers, single or multiple like the systems
 * select. Opening it lists the servers closest to the hub; typing searches Ookla's
 * server list.
 */
export function SpeedtestServerSelect({
	id,
	value,
	onChange,
	multiple,
	disabled,
}: {
	id: string
	value: SpeedtestServer[]
	onChange: (servers: SpeedtestServer[]) => void
	multiple?: boolean
	disabled?: boolean
}) {
	const { t } = useLingui()
	const [open, setOpen] = useState(false)
	const [search, setSearch] = useState("")
	const [servers, setServers] = useState<SpeedtestServer[]>([])
	const [loading, setLoading] = useState(false)
	const [error, setError] = useState("")
	const searchRef = useRef<HTMLInputElement>(null)
	const contentRef = useRef<HTMLDivElement>(null)
	const focusSearchOnMount = useCallback((node: HTMLInputElement | null) => {
		searchRef.current = node
		if (!node) return
		// Focus after the menu has completed its own initial focus handling.
		const frame = requestAnimationFrame(() => node.focus())
		return () => cancelAnimationFrame(frame)
	}, [])

	// Fetch servers when opened and after typing pauses. Stale responses are ignored.
	useEffect(() => {
		if (!open) return
		let cancelled = false
		const query = search.trim()
		setLoading(true)
		const timeout = setTimeout(
			() => {
				pb.send<SpeedtestServer[]>("/api/beszel/speedtest/servers", {
					query: query ? { search: query } : {},
					requestKey: null,
				})
					.then((result) => {
						if (cancelled) return
						setServers(result ?? [])
						setError("")
					})
					.catch((err: Error) => {
						if (cancelled) return
						setServers([])
						setError(err?.message || t`Failed to load servers.`)
					})
					.finally(() => {
						if (!cancelled) setLoading(false)
					})
			},
			query ? SEARCH_DEBOUNCE_MS : 0
		)
		return () => {
			cancelled = true
			clearTimeout(timeout)
		}
	}, [open, search, t])

	const query = search.trim()
	const listed = query ? servers : [AUTOMATIC_SERVER, ...servers]
	const selectedIds = new Set(value.map((server) => server.id))
	const allSelected = listed.every((server) => selectedIds.has(server.id))
	const anySelected = listed.some((server) => selectedIds.has(server.id))

	const serverLabel = (server: SpeedtestServer) =>
		server.id === 0 ? t`Automatic` : [server.name, server.location].filter(Boolean).join(" — ") || `#${server.id}`
	const label =
		value.length === 0 ? t`Select servers` : value.length === 1 ? serverLabel(value[0]) : t`${value.length} selected`

	const toggle = (server: SpeedtestServer, checked: boolean) => {
		// Picking a first specific server replaces the default Automatic selection.
		if (checked && server.id !== 0 && value.length === 1 && value[0].id === 0) {
			onChange([server])
			return
		}
		const rest = value.filter((selected) => selected.id !== server.id)
		onChange(checked ? [...rest, server] : rest)
	}

	const selectListed = (selected: boolean) => {
		const listedIds = new Set(listed.map((server) => server.id))
		const rest = value.filter((server) => !listedIds.has(server.id))
		onChange(selected ? [...rest, ...listed] : rest)
	}

	return (
		<DropdownMenu
			open={open}
			onOpenChange={(nextOpen) => {
				setOpen(nextOpen)
				setSearch("")
			}}
		>
			<DropdownMenuTrigger asChild>
				<Button
					id={id}
					disabled={disabled}
					type="button"
					variant="outline"
					className="relative w-full min-w-0 ps-10 pe-10 justify-start font-normal text-start"
				>
					<GlobeIcon className="size-3.5 absolute start-4 top-1/2 -translate-y-1/2 opacity-85" />
					<span className="truncate">{label}</span>
					<ChevronDownIcon className="size-4 absolute end-4 top-1/2 -translate-y-1/2 opacity-50" />
				</Button>
			</DropdownMenuTrigger>
			<DropdownMenuContent
				ref={contentRef}
				onKeyDown={(event) => {
					if (event.key === "Tab") {
						event.preventDefault()
						searchRef.current?.focus()
					}
				}}
				align="start"
				className="w-[var(--radix-dropdown-menu-trigger-width)] max-h-[min(22rem,var(--radix-dropdown-menu-content-available-height))] flex flex-col overflow-hidden"
			>
				<div className="shrink-0 border-b mb-1">
					<div className="flex items-center gap-2 px-2.5">
						<SearchIcon aria-hidden="true" className="size-4 shrink-0 text-muted-foreground" />
						<Input
							ref={focusSearchOnMount}
							value={search}
							onChange={(event) => setSearch(event.target.value)}
							placeholder={t`Search servers`}
							aria-label={t`Search servers`}
							className="h-10 min-w-0 rounded-none border-0 bg-transparent px-0 shadow-none focus-visible:ring-0 focus-visible:ring-offset-0"
							onKeyDown={(event) => {
								if (event.key === "Escape") return
								// Keep menu typeahead and form submission from consuming search input.
								event.stopPropagation()
								if (event.key === "Enter") event.preventDefault()
								if (event.key === "ArrowDown" || event.key === "ArrowUp" || event.key === "Tab") {
									event.preventDefault()
									const items = contentRef.current?.querySelectorAll<HTMLElement>(
										'[role^="menuitem"]:not([data-disabled])'
									)
									const index = event.key === "ArrowUp" || event.shiftKey ? (items?.length ?? 1) - 1 : 0
									items?.[index]?.focus()
								}
							}}
						/>
						{loading && <LoaderCircleIcon className="size-4 shrink-0 animate-spin text-muted-foreground" />}
					</div>
					{multiple && (
						<div className="flex flex-wrap items-center justify-between gap-x-3 gap-y-1 px-1 pb-1">
							<div className="flex items-center">
								<DropdownMenuItem
									className="px-1.5 py-1 text-xs text-muted-foreground"
									disabled={!listed.length || allSelected}
									onSelect={(event) => {
										event.preventDefault()
										selectListed(true)
									}}
								>
									{query ? <Trans>Select matches</Trans> : <Trans>Select all</Trans>}
								</DropdownMenuItem>
								<span aria-hidden="true" className="text-xs text-muted-foreground/50">
									·
								</span>
								<DropdownMenuItem
									className="px-1.5 py-1 text-xs text-muted-foreground"
									disabled={!anySelected}
									onSelect={(event) => {
										event.preventDefault()
										selectListed(false)
									}}
								>
									{query ? <Trans>Clear matches</Trans> : <Trans>Clear all</Trans>}
								</DropdownMenuItem>
							</div>
							<span className="px-1.5 text-xs tabular-nums text-muted-foreground">{t`${value.length} selected`}</span>
						</div>
					)}
				</div>
				<div className="min-h-0 overflow-y-auto">
					{listed.map((server) => (
						<ServerItem
							key={server.id}
							multiple={multiple}
							selected={selectedIds.has(server.id)}
							title={server.id === 0 ? t`Automatic` : server.name}
							description={server.id === 0 ? t`Pick the best server for each run` : server.location}
							onSelect={(checked) => (multiple ? toggle(server, checked) : onChange([server]))}
						/>
					))}
					{!loading && !error && servers.length === 0 && (
						<output className="block px-2.5 py-3 text-sm text-muted-foreground">
							<Trans>No servers found.</Trans>
						</output>
					)}
					{error && <p className="px-2.5 py-3 text-sm text-red-500">{error}</p>}
				</div>
			</DropdownMenuContent>
		</DropdownMenu>
	)
}

function ServerItem({
	multiple,
	selected,
	title,
	description,
	onSelect,
}: {
	multiple?: boolean
	selected: boolean
	title: string
	description: string
	onSelect: (checked: boolean) => void
}) {
	const content = (
		<div className="grid min-w-0">
			<span className="truncate">{title}</span>
			{description && <span className="truncate text-xs text-muted-foreground">{description}</span>}
		</div>
	)
	if (multiple) {
		return (
			<DropdownMenuCheckboxItem
				checked={selected}
				onSelect={(event) => event.preventDefault()}
				onCheckedChange={onSelect}
				className="group min-w-0 gap-2.5 py-2 ps-2.5"
				indicatorClassName="static size-4 shrink-0 rounded border border-input group-data-[state=checked]:border-primary group-data-[state=checked]:bg-primary group-data-[state=checked]:text-primary-foreground [&_svg]:size-3"
			>
				{content}
			</DropdownMenuCheckboxItem>
		)
	}
	return (
		<DropdownMenuItem className="min-w-0 gap-2.5 py-2 ps-2.5" onSelect={() => onSelect(true)}>
			<CheckIcon className={cn("size-4 shrink-0", !selected && "invisible")} />
			{content}
		</DropdownMenuItem>
	)
}
