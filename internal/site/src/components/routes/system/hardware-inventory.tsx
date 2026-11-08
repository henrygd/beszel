import { t } from "@lingui/core/macro"
import { Trans } from "@lingui/react/macro"
import { CircuitBoardIcon } from "lucide-react"
import { useEffect, useState } from "react"
import { Card, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Separator } from "@/components/ui/separator"
import { pb } from "@/lib/api"
import { formatShortDate } from "@/lib/utils"
import type { HardwareInventory } from "@/types"
import { HardwareSection as Section } from "./hardware-section"

const decimalSize = (bytes: number) =>
	bytes >= 1e12 ? `${(bytes / 1e12).toFixed(1)} TB` : `${Math.round(bytes / 1e9)} GB`

const join = (...parts: (string | number | undefined)[]) => parts.filter(Boolean).join(" ")

export default function HardwareInventoryCard({ systemId }: { systemId: string }) {
	const [data, setData] = useState<HardwareInventory | null>(null)
	const [error, setError] = useState<string | null>(null)

	useEffect(() => {
		let cancelled = false
		pb.send<HardwareInventory>("/api/beszel/hardware", { query: { system: systemId } })
			.then((result) => {
				if (cancelled) return
				setData(result)
				setError(null)
			})
			.catch((err) => {
				if (cancelled) return
				setError(err?.message || t`Failed to load hardware inventory`)
			})
		return () => {
			cancelled = true
		}
	}, [systemId])

	if (!data && !error) return null
	const inv = data
	const mono = (s?: string) => (s ? <span className="font-mono text-sm">{s}</span> : undefined)

	return (
		<Card className="@container w-full px-3 py-5 sm:py-6 sm:px-6">
			<CardHeader className="p-0 mb-4">
				<CardTitle className="mb-2 px-2 sm:px-1 flex items-center gap-2">
					<CircuitBoardIcon className="size-4" />
					<Trans>Hardware</Trans>
				</CardTitle>
				{inv && (
					<CardDescription className="px-2 sm:px-1 flex items-center flex-wrap">
						{join(inv.system.vendor, inv.system.product) || join(inv.board.vendor, inv.board.product)}
						<Separator orientation="vertical" className="h-4 mx-2 bg-primary/40" />
						<Trans>Collected {formatShortDate(new Date(inv.collected * 1000).toISOString())}</Trans>
					</CardDescription>
				)}
			</CardHeader>
			{error || !inv ? (
				<p className="px-2 sm:px-1 text-sm text-muted-foreground">{error}</p>
			) : (
				<div className="grid gap-5">
					<Section
						title={t`System`}
						head={[t`Component`, t`Vendor`, t`Model`, t`Version`, t`Serial`]}
						rows={[
							[
								t`System`,
								inv.system.vendor,
								inv.system.product,
								inv.system.version,
								mono(inv.system.serial ?? inv.system.uuid),
							],
							[t`Board`, inv.board.vendor, inv.board.product, inv.board.version, mono(inv.board.serial)],
							[t`BIOS`, inv.bios.vendor, inv.bios.version, inv.bios.date, undefined],
							[t`Chassis`, inv.chassis.vendor, inv.chassis.type, undefined, mono(inv.chassis.serial)],
							...(inv.cpus ?? []).map((c) => [
								t`CPU`,
								c.vendor,
								c.model,
								join(c.cores && `${c.cores}C`, c.threads && `${c.threads}T`, c.microcode && `µcode ${c.microcode}`),
								c.socket,
							]),
						]}
					/>
					<Section
						title={t`Security`}
						head={[t`Feature`, t`State`]}
						rows={[
							[t`Secure Boot`, inv.security?.secure_boot ?? t`Unknown`],
							[t`TPM`, inv.security?.tpm ?? t`Not detected`],
						]}
					/>
					<Section
						title={t`Batteries`}
						head={[t`Battery`, t`Model`, t`Technology`, t`Health`, t`Cycles`, t`Serial`]}
						rows={(inv.batteries ?? []).map((b) => [
							b.name,
							join(b.manufacturer, b.model) || undefined,
							b.technology,
							b.health_pct ? `${b.health_pct}%` : undefined,
							b.cycle_count || undefined,
							mono(b.serial),
						])}
					/>
					<Section
						title={t`Memory`}
						head={[t`Slot`, t`Size`, t`Type`, t`Speed`, t`Part number`, t`Serial`]}
						rows={(inv.memory ?? []).map((m) => [
							join(m.bank, m.locator),
							m.size_mb ? `${m.size_mb / 1024} GB` : t`Empty`,
							join(m.type, m.form_factor),
							m.configured_mts ? `${m.configured_mts} MT/s` : undefined,
							join(m.manufacturer, m.part_number) || undefined,
							mono(m.serial),
						])}
					/>
					<Section
						title={t`Disks`}
						head={[t`Device`, t`Model`, t`Size`, t`Bus`, t`Firmware`, t`Serial`]}
						rows={(inv.disks ?? []).map((d) => [
							mono(d.name),
							join(d.vendor, d.model),
							decimalSize(d.size_bytes),
							join(d.transport, d.rotational ? "HDD" : undefined),
							d.firmware,
							mono(d.serial),
						])}
					/>
					<Section
						title={t`Displays`}
						head={[t`Connector`, t`Monitor`, t`Year`, t`Size`, t`Serial`]}
						rows={(inv.displays ?? []).map((d) => [
							d.connector,
							join(d.manufacturer, d.name ?? d.product_code),
							d.year,
							d.width_cm ? `${d.width_cm} × ${d.height_cm} cm` : undefined,
							mono(d.serial),
						])}
					/>
					<Section
						title={t`USB devices`}
						head={[t`Device`, t`Manufacturer`, t`Type`, t`Speed`, t`ID`, t`Serial`]}
						rows={(inv.usb ?? []).map((u) => [
							u.name ?? u.product_name ?? `${u.vendor}:${u.product}`,
							u.manufacturer ?? u.vendor_name,
							u.kinds?.join(", "),
							u.speed_mbps ? `${u.speed_mbps} Mb/s` : undefined,
							mono(`${u.vendor}:${u.product}`),
							mono(u.serial),
						])}
					/>
					<Section
						title={t`Network`}
						head={[t`Interface`, t`Driver`, t`Link`, t`PCI`, t`MAC`]}
						rows={(inv.nics ?? []).map((n) => [
							mono(n.name),
							n.driver,
							n.speed_mbps ? `${n.speed_mbps} Mb/s` : t`Down`,
							mono(n.pci),
							mono(n.mac),
						])}
					/>
					<Section
						title={t`PCI devices`}
						head={[t`Address`, t`Class`, t`Device`, t`ID`, t`Driver`]}
						rows={(inv.pci ?? [])
							// bridges (class 06) are chipset plumbing, not installed hardware
							.filter((p) => !p.class.startsWith("06"))
							.map((p) => [
								mono(p.address),
								p.class_name ?? mono(p.class),
								join(p.vendor_name, p.device_name) || undefined,
								mono(`${p.vendor}:${p.device}`),
								p.driver,
							])}
					/>
					{inv.unavailable?.map((gap) => (
						<p key={gap.section} className="px-2 sm:px-1 text-sm text-muted-foreground">
							{gap.section}: {gap.reason}
						</p>
					))}
				</div>
			)}
		</Card>
	)
}
