import { Trans } from "@lingui/react/macro"
import { useStore } from "@nanostores/react"
import { $alerts, $systems, pb } from "@/lib/stores"
import { alertInfo } from "@/lib/utils"
import { Separator } from "@/components/ui/separator"
import { Card } from "@/components/ui/card"
import { Button } from "@/components/ui/button"
import type { AlertRecord, SystemRecord } from "@/types"
import React from "react"
import {
  Sheet,
  SheetContent,
  SheetHeader,
  SheetTitle,
  SheetDescription,
} from "@/components/ui/sheet"
import { toast } from "@/components/ui/use-toast"
import {
  DropdownMenu,
  DropdownMenuTrigger,
  DropdownMenuContent,
  DropdownMenuCheckboxItem,
} from "@/components/ui/dropdown-menu";
import { updateAlertsForSystems } from "@/components/alerts/alerts-system"
import { PlusIcon } from "lucide-react"
import MultiSystemAlertSheetContent from "@/components/alerts/alerts-multi-sheet"
import {
  Accordion,
  AccordionContent,
  AccordionItem,
  AccordionTrigger,
} from "@/components/ui/accordion"

function ConfiguredAlertsTab({ alerts, systems }: { alerts: AlertRecord[]; systems: SystemRecord[] }) {
	// Group alerts by type (name)
	const alertsByType: Record<string, AlertRecord[]> = React.useMemo(() => {
		const map: Record<string, AlertRecord[]> = {};
		for (const alert of alerts) {
			if (!map[alert.name]) map[alert.name] = [];
			map[alert.name].push(alert);
		}
		return map;
	}, [alerts]);

	// Sheet state for editing a group of alerts
	const [openSheet, setOpenSheet] = React.useState(false)
	const [editAlerts, setEditAlerts] = React.useState<AlertRecord[] | null>(null)
	const [editValue, setEditValue] = React.useState<number | null>(null)
	const [editMin, setEditMin] = React.useState<number | null>(null)

	// Sync edit state with alert group when Sheet opens
	React.useEffect(() => {
		if (editAlerts && openSheet) {
			setEditValue(editAlerts[0]?.value ?? null)
			setEditMin(editAlerts[0]?.min ?? null)
		}
	}, [editAlerts, openSheet])

	function openEditSheet(alerts: AlertRecord[]) {
		setEditAlerts(alerts)
		setOpenSheet(true)
	}

	function closeSheet() {
		setOpenSheet(false)
		setEditAlerts(null)
		setEditValue(null)
		setEditMin(null)
	}

	async function handleSave(alerts: AlertRecord[]) {
		if (!editValue || !editMin) return;
		const systemsToUpdate = systems.filter(s => alerts.some(a => a.system === s.id));
		const alertsBySystem = new Map(alerts.map(a => [a.system, a]));
		await updateAlertsForSystems({
			systems: systemsToUpdate,
			alertsBySystem,
			alertName: alerts[0].name,
			checked: true,
			value: editValue,
			min: editMin,
			userId: pb.authStore.model?.id || pb.authStore.record?.id || "",
			onAllDisabled: undefined,
			systemAlerts: alerts,
			allSystems: systems,
		})
		toast({
			title: "Alert updated",
			description: "Your alert configuration has been saved.",
		})
		closeSheet()
	}

	async function handleDelete(alerts: AlertRecord[]) {
		const confirmed = window.confirm(`Are you sure you want to delete this alert for all selected systems? This action cannot be undone.`);
		if (!confirmed) return;
		for (const alert of alerts) {
			await pb.collection("alerts").delete(alert.id);
		}
		toast({
			title: "Alert deleted",
			description: "Your alert configuration has been deleted.",
		});
		closeSheet();
	}

	const alertTypeOptions = Object.keys(alertInfo).map((key) => ({ label: alertInfo[key].name(), value: key }));
	const systemOptions = systems.map((s) => ({ label: s.name, value: s.id }));

	// --- FILTER STATE ---
	const [filterTypes, setFilterTypes] = React.useState<string[]>([]);
	const [filterSystems, setFilterSystems] = React.useState<string[]>([]);

	// --- FILTERED TYPES ---
	const filteredTypes = Object.keys(alertsByType).filter(type => {
		const typeMatch = filterTypes.length === 0 || filterTypes.includes(type);
		const systemMatch = filterSystems.length === 0 || alertsByType[type].some(a => filterSystems.includes(a.system));
		return typeMatch && systemMatch;
	});

	return (
		<div className="space-y-6">
			{/* FILTERS ROW */}
			<div className="flex flex-wrap gap-3 mb-2 items-center">
				{/* Alert Type Filter */}
				<DropdownMenu>
					<DropdownMenuTrigger asChild>
						<Button variant="outline" className="min-w-[180px] flex items-center justify-between">
							<span className="truncate">
								{filterTypes.length === 0
									? <Trans>All Types</Trans>
									: alertTypeOptions.filter(o => filterTypes.includes(o.value)).map(o => o.label).join(", ")}
							</span>
						</Button>
					</DropdownMenuTrigger>
					<DropdownMenuContent className="w-64 max-h-60 overflow-auto">
						{alertTypeOptions.map((option) => (
							<DropdownMenuCheckboxItem
								key={option.value}
								checked={filterTypes.includes(option.value)}
								onCheckedChange={(checked) => {
									if (checked) {
										setFilterTypes([...filterTypes, option.value]);
									} else {
										setFilterTypes(filterTypes.filter((v) => v !== option.value));
									}
								}}
							>
								{option.label}
							</DropdownMenuCheckboxItem>
						))}
					</DropdownMenuContent>
				</DropdownMenu>
				{/* System Filter */}
				<DropdownMenu>
					<DropdownMenuTrigger asChild>
						<Button variant="outline" className="min-w-[180px] flex items-center justify-between">
							<span className="truncate">
								{filterSystems.length === 0
									? <Trans>All Systems</Trans>
									: systemOptions.filter(o => filterSystems.includes(o.value)).map(o => o.label).join(", ")}
							</span>
						</Button>
					</DropdownMenuTrigger>
					<DropdownMenuContent className="w-64 max-h-60 overflow-auto">
						{systemOptions.map((option) => (
							<DropdownMenuCheckboxItem
								key={option.value}
								checked={filterSystems.includes(option.value)}
								onCheckedChange={(checked) => {
									if (checked) {
										setFilterSystems([...filterSystems, option.value]);
									} else {
										setFilterSystems(filterSystems.filter((v) => v !== option.value));
									}
								}}
							>
								{option.label}
							</DropdownMenuCheckboxItem>
						))}
					</DropdownMenuContent>
				</DropdownMenu>
			</div>
			{/* ALERT ACCORDION BY TYPE, GROUPED BY (min, value) */}
			{filteredTypes.length === 0 ? (
				<p className="text-muted-foreground"><Trans>No alerts configured.</Trans></p>
			) : (
				<Accordion type="single" collapsible className="w-full">
					{filteredTypes.map((type) => {
						// Group alerts of this type by (min, value)
						const groupMap: Record<string, AlertRecord[]> = {};
						for (const alert of alertsByType[type]) {
							const key = `${alert.min}|${alert.value}`;
							if (!groupMap[key]) groupMap[key] = [];
							groupMap[key].push(alert);
						}
						return (
							<AccordionItem key={type} value={type}>
								<AccordionTrigger>
									{alertInfo[type]?.name() ?? type}
								</AccordionTrigger>
								<AccordionContent className="flex flex-col gap-4 text-balance">
									{Object.entries(groupMap).map(([key, groupAlerts]) => {
										const min = groupAlerts[0].min;
										const value = groupAlerts[0].value;
										const systemNames = groupAlerts
											.map(alert => systems.find(s => s.id === alert.system)?.name ?? alert.system)
											.join(', ');
										return (
											<Card key={key} className="p-4 flex flex-col gap-2">
												<div className="flex justify-between items-center">
													<div>
														<div className="font-semibold">{systemNames}</div>
														<div className="text-sm text-muted-foreground">
															<Trans>Min</Trans>: <b>{min}</b> | <Trans>Value</Trans>: <b>{value}</b>
														</div>
													</div>
													<div className="flex gap-2">
														<Button size="sm" variant="outline" onClick={() => openEditSheet(groupAlerts)}>
															<Trans>Edit</Trans>
														</Button>
														<Button size="sm" variant="destructive" onClick={() => handleDelete(groupAlerts)}>
															<Trans>Delete</Trans>
														</Button>
													</div>
												</div>
											</Card>
										);
									})}
								</AccordionContent>
							</AccordionItem>
						);
					})}
				</Accordion>
			)}
			{/* Single Sheet instance for editing */}
			<Sheet open={openSheet} onOpenChange={open => !open ? closeSheet() : setOpenSheet(true)}>
				{editAlerts ? (
					<SheetContent side="right" className="flex flex-col h-full">
						<SheetHeader>
							<SheetTitle><Trans>Edit Alert</Trans></SheetTitle>
							<SheetDescription>Edit the alert configuration for these systems.</SheetDescription>
						</SheetHeader>
						<MultiSystemAlertSheetContent
							systems={systems}
							alerts={alerts}
							initialSystems={editAlerts.map(a => a.system)}
							onClose={closeSheet}
							singleAlertType={editAlerts[0]?.name}
							value={editValue ?? undefined}
							min={editMin ?? undefined}
							onValueChange={setEditValue}
							onMinChange={setEditMin}
						/>
						<Button className="mt-4 self-end" onClick={() => handleSave(editAlerts)}>
							<Trans>Save</Trans>
						</Button>
					</SheetContent>
				) : (
					openSheet && closeSheet(), null
				)}
			</Sheet>
		</div>
	);
}

export default function AlertsSettingsPage() {
	const alerts = useStore($alerts)
	const systems = useStore($systems)

	// Add Alert Sheet state
	const [addSheetOpen, setAddSheetOpen] = React.useState(false)
	const [addAlertSystems] = React.useState<string[]>([]);

	return (
		<div>
			<div className="flex items-center justify-between mb-4">
				<h3 className="text-xl font-medium">
					<Trans>Alerts</Trans>
				</h3>
				<Button variant="outline" onClick={() => setAddSheetOpen(true)}>
					<PlusIcon className="w-4 h-4 mr-2" />
					<Trans>Add Alert</Trans>
				</Button>
			</div>
			<p className="text-sm text-muted-foreground leading-relaxed mb-4">
				<Trans>Overview of all configured alerts, grouped by alert type and configuration.</Trans>
			</p>
			<Separator className="my-4" />
			<ConfiguredAlertsTab alerts={alerts} systems={systems} />

			{/* Add Alert Sheet */}
			<Sheet open={addSheetOpen} onOpenChange={setAddSheetOpen}>
				<SheetContent side="right" className="flex flex-col h-full w-[55em] max-w-3xl">
					<SheetHeader>
						<SheetTitle><Trans>Add Alert</Trans></SheetTitle>
						<SheetDescription>
							<Trans>Select systems and configure alerts for each type below.</Trans>
						</SheetDescription>
					</SheetHeader>
					<MultiSystemAlertSheetContent
						systems={systems}
						alerts={[]}
						initialSystems={addAlertSystems}
						onClose={() => setAddSheetOpen(false)}
						hideSystemSelector={false}
					/>
				</SheetContent>
			</Sheet>
		</div>
	)
} 