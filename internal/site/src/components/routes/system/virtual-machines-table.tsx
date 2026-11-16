import { Trans } from "@lingui/react/macro"
import { Badge, type BadgeProps } from "@/components/ui/badge"
import { Card, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table"
import { decimalString, formatBytes } from "@/lib/utils"
import { Unit } from "@/lib/enums"
import type { VMStats } from "@/types"

function StateLabel({ state }: { state: number }) {
	switch (state) {
		case 1:
			return <Trans>Running</Trans>
		case 2:
			return <Trans>Blocked</Trans>
		case 3:
			return <Trans>Paused</Trans>
		case 4:
			return <Trans>Shutting down</Trans>
		case 5:
			return <Trans>Shut off</Trans>
		case 6:
			return <Trans>Crashed</Trans>
		case 7:
			return <Trans>Suspended</Trans>
		default:
			return <Trans>Unknown</Trans>
	}
}

function stateVariant(state: number): BadgeProps["variant"] {
	if (state === 1) return "success"
	if (state === 3 || state === 7) return "warning"
	if (state === 5) return "secondary"
	return "danger"
}

function bytes(value: number) {
	const formatted = formatBytes(value, false, Unit.Bytes, false)
	return `${decimalString(formatted.value)} ${formatted.unit}`
}

export function VirtualMachinesTable({ machines }: { machines: VMStats[] }) {
	return (
		<Card className="w-full px-3 py-5 sm:py-6 sm:px-6">
			<CardHeader className="p-0 mb-3 sm:mb-4">
				<CardTitle className="mb-2">
					<Trans>Virtual Machines</Trans>
				</CardTitle>
				<CardDescription>
					<Trans>Libvirt domains visible through the configured read-only connection.</Trans>
				</CardDescription>
			</CardHeader>
			<Table>
				<TableHeader>
					<TableRow>
						<TableHead>
							<Trans>Name</Trans>
						</TableHead>
						<TableHead>
							<Trans>State</Trans>
						</TableHead>
						<TableHead className="text-end">
							<Trans>CPU</Trans>
						</TableHead>
						<TableHead className="text-end">
							<Trans>Memory</Trans>
						</TableHead>
						<TableHead className="text-end">
							<Trans>Disk allocation</Trans>
						</TableHead>
						<TableHead className="text-end">
							<Trans>vCPUs</Trans>
						</TableHead>
						<TableHead className="text-end">
							<Trans>Autostart</Trans>
						</TableHead>
					</TableRow>
				</TableHeader>
				<TableBody>
					{machines.map((machine) => (
						<TableRow key={machine.id}>
							<TableCell>
								<div className="font-medium">{machine.n}</div>
								<div className="text-xs text-muted-foreground font-mono">{machine.id}</div>
							</TableCell>
							<TableCell>
								<Badge variant={stateVariant(machine.s)}>
									<StateLabel state={machine.s} />
								</Badge>
							</TableCell>
							<TableCell className="text-end tabular-nums">{decimalString(machine.c)}%</TableCell>
							<TableCell className="text-end tabular-nums">
								{machine.mf && machine.mt !== undefined
									? `${bytes((machine.m ?? 0) * 1024 * 1024)} / ${bytes(machine.mt * 1024 * 1024)}`
									: "—"}
							</TableCell>
							<TableCell className="text-end tabular-nums">
								{machine.dc !== undefined ? `${bytes(machine.da ?? 0)} / ${bytes(machine.dc)}` : "—"}
							</TableCell>
							<TableCell className="text-end tabular-nums">{machine.v ?? "—"}</TableCell>
							<TableCell className="text-end">{machine.a ? <Trans>Yes</Trans> : <Trans>No</Trans>}</TableCell>
						</TableRow>
					))}
				</TableBody>
			</Table>
		</Card>
	)
}
