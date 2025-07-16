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
  SheetFooter,
  SheetClose,
} from "@/components/ui/sheet"
import { Input } from "@/components/ui/input"
import { toast } from "@/components/ui/use-toast"
import {
  DropdownMenu,
  DropdownMenuTrigger,
  DropdownMenuContent,
  DropdownMenuCheckboxItem,
} from "@/components/ui/dropdown-menu";
import { AlertDialog, AlertDialogAction, AlertDialogCancel, AlertDialogContent, AlertDialogDescription, AlertDialogFooter, AlertDialogHeader, AlertDialogTitle, AlertDialogTrigger } from "@/components/ui/alert-dialog";
import { X } from "lucide-react"
import { SystemAlert, updateAlertsForSystems } from "@/components/alerts/alerts-system"
import { PlusIcon } from "lucide-react"
import SystemsDropdown from "@/components/ui/systems-dropdown"
import MultiSystemAlertSheetContent from "@/components/alerts/alerts-multi-sheet"

export default function AlertsSettingsPage() {
	const alerts = useStore($alerts)
	const systems = useStore($systems)

	// Add Alert Sheet state
	const [addSheetOpen, setAddSheetOpen] = React.useState(false)
	const alertTypeOptions = Object.keys(alertInfo).map((key) => ({ label: alertInfo[key].name(), value: key }))
	// Add a single addAlertSystems state (default empty array)
	const [addAlertSystems, setAddAlertSystems] = React.useState<string[]>([]);
	// Remove addAlertType and addAlertSystems state. Instead, for each alert type, render a SystemAlert box with its own systems dropdown (default empty).
	// In the Add Alert Sheet, replace the type dropdown and single SystemAlert with:
	// {alertTypeOptions.map((typeOption) => {
	//   const [selectedSystems, setSelectedSystems] = React.useState<string[]>([]);
	//   const allAlerts = alerts.filter(a => a.name === typeOption.value && selectedSystems.includes(a.system));
	//   const system = systems.find(s => selectedSystems[0]) || systems[0];
	//   const data = {
	//     name: typeOption.value,
	//     alert: alertInfo[typeOption.value],
	//     system,
	//   };
	//   return (
	//     <div key={typeOption.value} className="mb-6">
	//       <label className="block text-sm font-medium mb-1"><Trans>Systems for {typeOption.label}</Trans></label>
	//       <SystemsDropdown
	//         options={systems.map(s => ({ label: s.name, value: s.id }))}
	//         value={selectedSystems}
	//         onChange={setSelectedSystems}
	//       />
	//       <SystemAlert
	//         key={typeOption.value + selectedSystems.join(",")}
	//         system={system}
	//         data={data}
	//         systemAlerts={allAlerts}
	//         onAllDisabled={() => setAddSheetOpen(false)}
	//       />
	//     </div>
	//   );
	// })}

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

function ConfiguredAlertsTab({ alerts, systems }: { alerts: AlertRecord[]; systems: SystemRecord[] }) {
	// Group alerts by name, value, and min
	type GroupKey = string;
	type Group = { name: string; value: number; min: number; alerts: AlertRecord[] };
	const groupMap = new Map<GroupKey, Group>();

	for (const alert of alerts) {
		const key = `${alert.name}|${alert.value}|${alert.min}`;
		if (!groupMap.has(key)) {
			groupMap.set(key, { name: alert.name, value: alert.value, min: alert.min, alerts: [] });
		}
		groupMap.get(key)!.alerts.push(alert);
	}

	const groupedAlerts = Array.from(groupMap.values());

	// Move status alert group to the top
	const statusIdx = groupedAlerts.findIndex(g => g.name === "status");
	let sortedGroupedAlerts = groupedAlerts;
	if (statusIdx > 0) {
		const statusGroup = groupedAlerts[statusIdx];
		sortedGroupedAlerts = [
			statusGroup,
			...groupedAlerts.slice(0, statusIdx),
			...groupedAlerts.slice(statusIdx + 1),
		];
	}

	const alertOrder = ["Status", "CPU", "Memory", "Disk", "Bandwidth", "LoadAvg5", "LoadAvg15"];
	sortedGroupedAlerts = [...sortedGroupedAlerts].sort((a, b) => {
		const aIdx = alertOrder.indexOf(a.name);
		const bIdx = alertOrder.indexOf(b.name);
		return (aIdx === -1 ? 999 : aIdx) - (bIdx === -1 ? 999 : bIdx);
	});

	// Sheet state
	const [openSheet, setOpenSheet] = React.useState(false)
	const [editGroupIdx, setEditGroupIdx] = React.useState<number | null>(null)
	const [editValue, setEditValue] = React.useState<number | null>(null)
	const [editMin, setEditMin] = React.useState<number | null>(null)
	const [editSystems, setEditSystems] = React.useState<string[]>([])

	// Sync edit state with group when Sheet opens
	React.useEffect(() => {
		if (editGroupIdx !== null && openSheet) {
			const group = sortedGroupedAlerts[editGroupIdx]
			if (group) {
				setEditValue(group.value)
				setEditMin(group.min)
				setEditSystems(group.alerts.map(a => a.system))
				console.log("Setting editSystems to:", group.alerts.map(a => a.system));
			}
		}
	}, [editGroupIdx, openSheet])

	function openEditSheet(idx: number) {
		setEditGroupIdx(idx)
		setOpenSheet(true)
	}

	function closeSheet() {
		setOpenSheet(false)
		setEditGroupIdx(null)
		setEditValue(null)
		setEditMin(null)
		setEditSystems([])
	}

	async function handleSave(group: Group) {
		if (!editValue || !editMin || editSystems.length === 0) return;
		const prevSystems = group.alerts.map(a => a.system)
		const toAdd = editSystems.filter(id => !prevSystems.includes(id))
		const toRemove = prevSystems.filter(id => !editSystems.includes(id))
		const toUpdate = editSystems.filter(id => prevSystems.includes(id))

		const alertsBySystem = new Map(group.alerts.map(a => [a.system, a]))
		const userId = pb.authStore.model?.id || pb.authStore.record?.id || ""

		// Batch update for updated systems (value/min changes)
		if (toUpdate.length > 0) {
			await updateAlertsForSystems({
				systems: systems.filter(s => toUpdate.includes(s.id)),
				alertsBySystem,
				alertName: group.name,
				checked: true,
				value: editValue,
				min: editMin,
				userId,
				onAllDisabled: undefined,
				systemAlerts: group.alerts,
				allSystems: systems,
			})
		}
		// Batch create for added systems
		if (toAdd.length > 0) {
			await updateAlertsForSystems({
				systems: systems.filter(s => toAdd.includes(s.id)),
				alertsBySystem,
				alertName: group.name,
				checked: true,
				value: editValue,
				min: editMin,
				userId,
				onAllDisabled: undefined,
				systemAlerts: group.alerts,
				allSystems: systems,
			})
		}
		// Batch remove for removed systems
		if (toRemove.length > 0) {
			await updateAlertsForSystems({
				systems: systems.filter(s => toRemove.includes(s.id)),
				alertsBySystem,
				alertName: group.name,
				checked: false,
				value: editValue,
				min: editMin,
				userId,
				onAllDisabled: undefined,
				systemAlerts: group.alerts,
				allSystems: systems,
			})
		}

		toast({
			title: "Alert updated",
			description: "Your alert configuration has been saved.",
		})
		closeSheet()
		// Optionally, refresh alerts from backend if needed
	}

	async function handleDelete(group: Group) {
		const confirmed = window.confirm(`Are you sure you want to delete this alert for all selected systems? This action cannot be undone.`);
		if (!confirmed) return;

		const systemIdsToDelete = group.alerts.map(a => a.system);
		for (const systemId of systemIdsToDelete) {
			const alert = group.alerts.find(a => a.system === systemId);
			if (alert) {
				await pb.collection("alerts").delete(alert.id);
			}
		}
		toast({
			title: "Alert deleted",
			description: "Your alert configuration has been deleted.",
		});
		closeSheet();
	}

	// --- FILTER STATE ---
	const [filterTypes, setFilterTypes] = React.useState<string[]>([]);
	const [filterSystems, setFilterSystems] = React.useState<string[]>([]);

	// --- FILTER OPTIONS ---
	const alertTypeOptions = Object.keys(alertInfo).map((key) => ({ label: alertInfo[key].name(), value: key }));
	const systemOptions = systems.map((s) => ({ label: s.name, value: s.id }));

	// --- FILTERED GROUPS ---
	let filteredGroupedAlerts = sortedGroupedAlerts.filter((group) => {
		// Type filter: if any selected, must match
		const typeMatch = filterTypes.length === 0 || filterTypes.includes(group.name);
		// System filter: if any selected, at least one alert in group must match
		const systemMatch = filterSystems.length === 0 || group.alerts.some(a => filterSystems.includes(a.system));
		return typeMatch && systemMatch;
	});

	console.log("editSystems:", editSystems);
	console.log("MultiSelect options:", systems.map(s => ({ label: s.name, value: s.id })));

	// Defensive check for currentAlertInfo and isStatusAlert
	const currentGroup = editGroupIdx !== null ? sortedGroupedAlerts[editGroupIdx] : undefined;
	const currentAlertInfo = currentGroup ? alertInfo[currentGroup.name] : undefined;

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
			{/* ALERT CARDS */}
			{filteredGroupedAlerts.length === 0 ? (
				<p className="text-muted-foreground"><Trans>No alerts configured.</Trans></p>
			) : (
				filteredGroupedAlerts.map((group, idx) => {
					const isStatusAlert = !!alertInfo[group.name]?.singleDesc;
					return (
						<Card key={group.name + group.value + group.min + idx} className="p-3 pl-5">
							<div className="flex items-center justify-between mb-1">
								<h4 className="font-semibold text-base">{alertInfo[group.name]?.name() ?? group.name}</h4>
								<div className="flex gap-1 items-center">
									<Button size="sm" variant="outline" onClick={() => openEditSheet(idx)}>
										<Trans>Edit</Trans>
									</Button>
									<Button size="sm" variant="destructive" onClick={() => handleDelete(group)}>
										<Trans>Delete</Trans>
									</Button>
								</div>
							</div>
							<div className="flex flex-wrap items-center gap-3 text-sm">
								{!isStatusAlert && (
									<span><Trans>Average exceeds</Trans>: <b>{group.value}</b></span>
								)}
								<span><Trans>Duration (minutes)</Trans>: <b>{group.min}</b></span>
								<span>
									<Trans>Systems</Trans>:{" "}
									{group.alerts.map((alert, i) => {
										const system = systems.find((s: SystemRecord) => s.id === alert.system);
										return (
											<span key={alert.id}>
												{system?.name ?? alert.system}{i < group.alerts.length - 1 ? ', ' : ''}
											</span>
										);
									})}
								</span>
							</div>
						</Card>
					);
				})
			)}
			{/* Single Sheet instance for editing */}
			<Sheet open={openSheet} onOpenChange={open => !open ? closeSheet() : setOpenSheet(true)}>
				<SheetContent side="right" className="flex flex-col h-full">
					{editGroupIdx !== null && currentGroup ? (
						<>
							<SheetHeader>
								<SheetTitle><Trans>Edit Alert</Trans></SheetTitle>
							</SheetHeader>
							<MultiSystemAlertSheetContent
								key={editGroupIdx}
								systems={systems}
								alerts={alerts}
								initialSystems={currentGroup.alerts.map(a => a.system)}
								onClose={closeSheet}
								singleAlertType={currentGroup.name}
								value={editValue ?? undefined}
								min={editMin ?? undefined}
								onValueChange={setEditValue}
								onMinChange={setEditMin}
							/>
						</>
					) : (
						// If the group is gone, close the Sheet
						openSheet && closeSheet(), null
					)}
				</SheetContent>
			</Sheet>
		</div>
	);
} 