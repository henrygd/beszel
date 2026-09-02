import { t } from "@lingui/core/macro"
import { CpuIcon, HeartPulseIcon, MemoryStickIcon, ServerIcon } from "lucide-react"
import type { RecordSubscription } from "pocketbase"
import { EthernetIcon } from "@/components/ui/icons"
import { $containerAlerts } from "@/lib/stores"
import type { ContainerAlertInfo, ContainerAlertRecord } from "@/types"
import { pb } from "./api"

/** Container alert info for each alert type */
export const containerAlertInfo: Record<string, ContainerAlertInfo> = {
    Status: {
        name: () => t`Status`,
        unit: "",
        icon: ServerIcon,
        desc: () => t`Triggers when container status changes`,
        singleDesc: () => `${t`Container`} ${t`Stopped`}`,
    },
    CPU: {
        name: () => t`CPU Usage`,
        unit: "%",
        icon: CpuIcon,
        desc: () => t`Triggers when CPU usage exceeds a threshold`,
    },
    Memory: {
        name: () => t`Memory Usage`,
        unit: "%",
        icon: MemoryStickIcon,
        desc: () => t`Triggers when memory usage exceeds a threshold`,
    },
    Network: {
        name: () => t`Network`,
        unit: " MB/s",
        icon: EthernetIcon,
        desc: () => t`Triggers when combined up/down exceeds a threshold`,
        max: 125,
    },
    Health: {
        name: () => t`Health Status`,
        unit: "",
        icon: HeartPulseIcon,
        desc: () => t`Triggers when container health status changes`,
        singleDesc: () => `${t`Container`} ${t`Unhealthy`}`,
    },
}

class ContainerAlertManager {
    private unsubscribeFn?: () => void

    /**
     * Subscribe to container alert updates
     */
    async subscribe() {
        if (this.unsubscribeFn) {
            return
        }

        // Fetch initial container alerts
        try {
            const alerts = await pb.collection("container_alerts").getFullList<ContainerAlertRecord>()
            this.updateStore(alerts)
        } catch (e) {
            console.error("Failed to fetch container alerts:", e)
        }

        // Subscribe to real-time updates
        this.unsubscribeFn = await pb
            .collection("container_alerts")
            .subscribe<ContainerAlertRecord>("*", this.handleAlertUpdate)
    }

    /**
     * Unsubscribe from container alert updates
     */
    unsubscribe() {
        if (this.unsubscribeFn) {
            this.unsubscribeFn()
            this.unsubscribeFn = undefined
        }
    }

    /**
     * Handle real-time alert updates
     */
    private handleAlertUpdate = (e: RecordSubscription<ContainerAlertRecord>) => {
        const { action, record } = e

        if (action === "delete") {
            this.deleteFromStore(record)
        } else {
            this.updateStore([record])
        }
    }

    /**
     * Update store with alert records
     */
    private updateStore(alerts: ContainerAlertRecord[]) {
        const currentAlerts = $containerAlerts.get()

        for (const alert of alerts) {
            if (!currentAlerts[alert.system]) {
                currentAlerts[alert.system] = new Map()
            }
            if (!currentAlerts[alert.system].get(alert.container)) {
                currentAlerts[alert.system].set(alert.container, new Map())
            }
            currentAlerts[alert.system].get(alert.container)!.set(alert.name, alert)
        }

        $containerAlerts.set(currentAlerts)
    }

    /**
     * Delete alert from store
     */
    private deleteFromStore(alert: ContainerAlertRecord) {
        const currentAlerts = $containerAlerts.get()

        if (currentAlerts[alert.system]?.get(alert.container)?.has(alert.name)) {
            currentAlerts[alert.system].get(alert.container)!.delete(alert.name)

            // Clean up empty maps
            if (currentAlerts[alert.system].get(alert.container)!.size === 0) {
                currentAlerts[alert.system].delete(alert.container)
            }
            if (currentAlerts[alert.system].size === 0) {
                delete currentAlerts[alert.system]
            }

            $containerAlerts.set(currentAlerts)
        }
    }

    /**
     * Create or update container alerts
     */
    async upsert(
        systems: string[],
        containers: string[],
        name: string,
        value: number,
        min: number,
        overwrite = false
    ) {
        return pb.send("/api/beszel/user-container-alerts", {
            method: "POST",
            body: {
                systems,
                containers,
                name,
                value,
                min,
                overwrite,
            },
        })
    }

    /**
     * Delete container alerts
     */
    async delete(systems: string[], containers: string[], name: string) {
        return pb.send("/api/beszel/user-container-alerts", {
            method: "DELETE",
            body: {
                systems,
                containers,
                name,
            },
        })
    }

    /**
     * Clear all container alerts from store
     */
    clear() {
        $containerAlerts.set({})
    }
}

export const containerAlertManager = new ContainerAlertManager()
