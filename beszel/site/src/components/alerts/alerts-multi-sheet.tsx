import React, { useState, useRef, useEffect } from "react"
import { alertInfo } from "@/lib/utils"
import { SystemAlert, updateAlertsForSystems } from "./alerts-system"
import { pb } from "@/lib/stores"
import SystemsDropdown from "@/components/ui/systems-dropdown"
import { Trans } from "@lingui/react/macro"
import type { SystemRecord, AlertRecord } from "@/types"

interface Props {
  systems: SystemRecord[];
  alerts: AlertRecord[];
  initialSystems: string[];
  onClose: () => void;
  singleAlertType?: string;
  value?: number;
  min?: number;
  onValueChange?: (v: number) => void;
  onMinChange?: (v: number) => void;
  hideSystemSelector?: boolean;
}

export default function MultiSystemAlertSheetContent({
  systems,
  alerts,
  initialSystems,
  onClose,
  singleAlertType,
  value,
  min,
  onValueChange,
  onMinChange,
  hideSystemSelector,
}: Props) {
  const [selectedSystems, setSelectedSystems] = useState<string[]>(initialSystems)
  const alertTypeOptions = singleAlertType
    ? [{ label: alertInfo[singleAlertType].name(), value: singleAlertType }]
    : Object.keys(alertInfo).map((key) => ({ label: alertInfo[key].name(), value: key }))

  // Track previous selected systems to detect additions
  const prevSelectedSystemsRef = React.useRef<string[]>(initialSystems)

  React.useEffect(() => {
    const prev = prevSelectedSystemsRef.current
    const added = selectedSystems.filter(id => !prev.includes(id))
    const removed = prev.filter(id => !selectedSystems.includes(id))
    prevSelectedSystemsRef.current = selectedSystems
    if (added.length > 0) {
      alertTypeOptions.forEach(typeOption => {
        const alertName = typeOption.value
        const enabled = alerts.some(a => a.name === alertName && selectedSystems.includes(a.system)) || !!singleAlertType
        if (enabled) {
          const refAlert = alerts.find(a => a.name === alertName && selectedSystems.includes(a.system))
          const v = value ?? refAlert?.value ?? alertInfo[alertName].start ?? 80
          const m = min ?? refAlert?.min ?? alertInfo[alertName].min ?? 1
          const alertsBySystem = new Map(alerts.filter(a => a.name === alertName && selectedSystems.includes(a.system)).map(a => [a.system, a]))
          updateAlertsForSystems({
            systems: systems.filter(s => added.includes(s.id)),
            alertsBySystem,
            alertName,
            checked: true,
            value: v,
            min: m,
            userId: pb.authStore.model?.id || pb.authStore.record?.id || "",
            onAllDisabled: undefined,
            systemAlerts: alerts,
            allSystems: systems,
          })
        }
      })
    }
    if (removed.length > 0) {
      alertTypeOptions.forEach(typeOption => {
        const alertName = typeOption.value
        const enabled = alerts.some(a => a.name === alertName && prev.includes(a.system)) || !!singleAlertType
        if (enabled) {
          const refAlert = alerts.find(a => a.name === alertName && prev.includes(a.system))
          const v = value ?? refAlert?.value ?? alertInfo[alertName].start ?? 80
          const m = min ?? refAlert?.min ?? alertInfo[alertName].min ?? 1
          const alertsBySystem = new Map(alerts.filter(a => a.name === alertName && prev.includes(a.system)).map(a => [a.system, a]))
          updateAlertsForSystems({
            systems: systems.filter(s => removed.includes(s.id)),
            alertsBySystem,
            alertName,
            checked: false,
            value: v,
            min: m,
            userId: pb.authStore.model?.id || pb.authStore.record?.id || "",
            onAllDisabled: undefined,
            systemAlerts: alerts,
            allSystems: systems,
          })
        }
      })
    }
  }, [selectedSystems, alerts, systems, value, min, singleAlertType])

  // Add useEffect to trigger batch update on selection change
  React.useEffect(() => {
    triggerBatchUpdate(selectedSystems)
  }, [selectedSystems])

  function triggerBatchUpdate(newSelection: string[]) {
    // For now, just log the new selection
    console.log('Batch update triggered for systems:', newSelection)
  }

  return (
    <>
      {!hideSystemSelector && (
        <div className="mb-6">
          <label className="block text-sm font-medium mb-1"><Trans>Systems</Trans></label>
          <SystemsDropdown
            options={systems.map(s => ({ label: s.name, value: s.id }))}
            value={selectedSystems}
            onChange={setSelectedSystems}
          />
        </div>
      )}
      {selectedSystems.length === 0 ? (
        <div className="text-muted-foreground text-center py-8">
          <Trans>Select one or more systems to configure alerts.</Trans>
        </div>
      ) : (
        <div className="grid gap-3 mt-4 overflow-y-auto">
          {alertTypeOptions.map((typeOption) => {
            const selectedSystemObjs = systems.filter(s => selectedSystems.includes(s.id));
            const allAlerts = alerts.filter(a => a.name === typeOption.value && selectedSystems.includes(a.system));
            const data = {
              name: typeOption.value,
              alert: alertInfo[typeOption.value],
              system: selectedSystemObjs[0] || systems[0],
            };
            // If singleAlertType, show value/min fields controlled by parent
            return (
              <div key={typeOption.value + selectedSystems.join(",") + (singleAlertType || "") }>
                {singleAlertType && (
                  <div className="flex gap-4 mb-2">
                    {/* Remove the Value and Min inputs, let SystemAlert's sliders handle them */}
                  </div>
                )}
                <SystemAlert
                  systems={selectedSystemObjs}
                  data={data}
                  systemAlerts={allAlerts}
                  onAllDisabled={onClose}
                />
              </div>
            );
          })}
        </div>
      )}
    </>
  )
}
