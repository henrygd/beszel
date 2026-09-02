import { t } from "@lingui/core/macro"
import { Plural, Trans } from "@lingui/react/macro"
import { useStore } from "@nanostores/react"
import { getPagePath } from "@nanostores/router"
import { BoxIcon, GlobeIcon } from "lucide-react"
import { lazy, memo, Suspense, useMemo, useState } from "react"
import { $router, Link } from "@/components/router"
import { Checkbox } from "@/components/ui/checkbox"
import { DialogDescription, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import { Switch } from "@/components/ui/switch"
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs"
import { toast } from "@/components/ui/use-toast"
import { containerAlertInfo } from "@/lib/container-alerts"
import { pb } from "@/lib/api"
import { $containerAlerts } from "@/lib/stores"
import { cn, debounce } from "@/lib/utils"
import type { ContainerAlertInfo, ContainerAlertRecord, ContainerRecord } from "@/types"

const Slider = lazy(() => import("@/components/ui/slider"))

const endpoint = "/api/beszel/user-container-alerts"

const alertDebounce = 100

const alertKeys = Object.keys(containerAlertInfo) as (keyof typeof containerAlertInfo)[]

const failedUpdateToast = (error: unknown) => {
    console.error(error)
    toast({
        title: t`Failed to update alert`,
        description: t`Please check logs for more details.`,
        variant: "destructive",
    })
}

/** Create or update container alerts */
const upsertContainerAlerts = debounce(
    async ({
        name,
        value,
        min,
        systems,
        containers,
    }: {
        name: string
        value: number
        min: number
        systems: string[]
        containers: string[]
    }) => {
        try {
            await pb.send<{ success: boolean }>(endpoint, {
                method: "POST",
                body: { name, value, min, systems, containers, overwrite: true },
            })
        } catch (error) {
            failedUpdateToast(error)
        }
    },
    alertDebounce
)

/** Delete container alerts */
const deleteContainerAlerts = debounce(
    async ({ name, systems, containers }: { name: string; systems: string[]; containers: string[] }) => {
        try {
            await pb.send<{ success: boolean }>(endpoint, {
                method: "DELETE",
                body: { name, systems, containers },
            })
        } catch (error) {
            failedUpdateToast(error)
        }
    },
    alertDebounce
)

export const ContainerAlertDialogContent = memo(function ContainerAlertDialogContent({
    systemId,
    container,
}: {
    systemId: string
    container: ContainerRecord
}) {
    const alerts = useStore($containerAlerts)
    const [overwriteExisting, setOverwriteExisting] = useState<boolean | "indeterminate">(false)
    const [currentTab, setCurrentTab] = useState("container")

    const containerAlerts = alerts[systemId]?.get(container.id) ?? new Map()

    // Keep a copy of alerts when we switch to global tab
    const alertsWhenGlobalSelected = useMemo(() => {
        return currentTab === "global" ? structuredClone(alerts) : alerts
    }, [currentTab])

    return (
        <>
            <DialogHeader>
                <DialogTitle className="text-xl">
                    <Trans>Container Alerts</Trans>
                </DialogTitle>
                <DialogDescription>
                    <Trans>
                        See{" "}
                        <Link href={getPagePath($router, "settings", { name: "notifications" })} className="link">
                            notification settings
                        </Link>{" "}
                        to configure how you receive alerts.
                    </Trans>
                </DialogDescription>
            </DialogHeader>
            <Tabs defaultValue="container" onValueChange={setCurrentTab}>
                <TabsList className="mb-1 -mt-0.5">
                    <TabsTrigger value="container">
                        <BoxIcon className="me-2 h-3.5 w-3.5" />
                        <span className="truncate max-w-60">{container.name}</span>
                    </TabsTrigger>
                    <TabsTrigger value="global">
                        <GlobeIcon className="me-1.5 h-3.5 w-3.5" />
                        <Trans>All Containers</Trans>
                    </TabsTrigger>
                </TabsList>
                <TabsContent value="container">
                    <div className="grid gap-3">
                        {alertKeys.map((name) => (
                            <ContainerAlertContent
                                key={name}
                                alertKey={name}
                                data={containerAlertInfo[name as keyof typeof containerAlertInfo]}
                                alert={containerAlerts.get(name)}
                                systemId={systemId}
                                container={container}
                            />
                        ))}
                    </div>
                </TabsContent>
                <TabsContent value="global">
                    <label
                        htmlFor="ovw"
                        className="mb-3 flex gap-2 items-center justify-center cursor-pointer border rounded-sm py-3 px-4 border-destructive text-destructive font-semibold text-sm"
                    >
                        <Checkbox
                            id="ovw"
                            className="text-destructive border-destructive data-[state=checked]:bg-destructive"
                            checked={overwriteExisting}
                            onCheckedChange={setOverwriteExisting}
                        />
                        <Trans>Overwrite existing alerts</Trans>
                    </label>
                    <div className="grid gap-3">
                        {alertKeys.map((name) => (
                            <ContainerAlertContent
                                key={name}
                                alertKey={name}
                                systemId={systemId}
                                container={container}
                                alert={containerAlerts.get(name)}
                                data={containerAlertInfo[name as keyof typeof containerAlertInfo]}
                                global={true}
                                overwriteExisting={!!overwriteExisting}
                                initialAlertsState={alertsWhenGlobalSelected}
                            />
                        ))}
                    </div>
                </TabsContent>
            </Tabs>
        </>
    )
})

export function ContainerAlertContent({
    alertKey,
    data: alertData,
    systemId,
    container,
    alert,
    global = false,
    overwriteExisting = false,
    initialAlertsState = {},
}: {
    alertKey: string
    data: ContainerAlertInfo
    systemId: string
    container: ContainerRecord
    alert?: ContainerAlertRecord
    global?: boolean
    overwriteExisting?: boolean
    initialAlertsState?: Record<string, Map<string, Map<string, ContainerAlertRecord>>>
}) {
    const { name } = alertData

    const singleDescription = alertData.singleDesc?.()

    const [checked, setChecked] = useState(global ? false : !!alert)
    const [min, setMin] = useState(alert?.min || 10)
    const [value, setValue] = useState(alert?.value || (singleDescription ? 0 : alertData.start ?? 80))

    const Icon = alertData.icon

    /** Get container ids to update */
    function getContainerIds(): string[] {
        // if not global, update only the current container
        if (!global) {
            return [container.id]
        }
        // if global, we need to get all containers for this system
        // For now, we'll just use the current container
        // In a real implementation, you'd fetch all containers for the system
        return [container.id]
    }

    function sendUpsert(min: number, value: number) {
        const containers = getContainerIds()
        containers.length &&
            upsertContainerAlerts({
                name: alertKey,
                value,
                min,
                systems: [systemId],
                containers,
            })
    }

    return (
        <div className="rounded-lg border border-muted-foreground/15 hover:border-muted-foreground/20 transition-colors duration-100 group">
            <label
                htmlFor={`c${name}`}
                className={cn("flex flex-row items-center justify-between gap-4 cursor-pointer p-4", {
                    "pb-0": checked,
                })}
            >
                <div className="grid gap-1 select-none">
                    <p className="font-semibold flex gap-3 items-center">
                        <Icon className="h-4 w-4 opacity-85" /> {alertData.name()}
                    </p>
                    {!checked && <span className="block text-sm text-muted-foreground">{alertData.desc()}</span>}
                </div>
                <Switch
                    id={`c${name}`}
                    checked={checked}
                    onCheckedChange={(newChecked) => {
                        setChecked(newChecked)
                        if (newChecked) {
                            // if alert checked, create or update alert
                            sendUpsert(min, value)
                        } else {
                            // if unchecked, delete alert
                            deleteContainerAlerts({ name: alertKey, systems: [systemId], containers: getContainerIds() })
                            // when force deleting all alerts of a type, also remove them from initialAlertsState
                            if (overwriteExisting) {
                                for (const systemAlerts of Object.values(initialAlertsState)) {
                                    for (const containerAlerts of systemAlerts.values()) {
                                        containerAlerts.delete(alertKey)
                                    }
                                }
                            }
                        }
                    }}
                />
            </label>
            {checked && (
                <div className="grid sm:grid-cols-2 mt-1.5 gap-5 px-4 pb-5 tabular-nums text-muted-foreground">
                    <Suspense fallback={<div className="h-10" />}>
                        {!singleDescription && (
                            <div>
                                <p id={`v${name}`} className="text-sm block h-8">
                                    {alertData.invert ? (
                                        <Trans>
                                            Average drops below{" "}
                                            <strong className="text-foreground">
                                                {value}
                                                {alertData.unit}
                                            </strong>
                                        </Trans>
                                    ) : (
                                        <Trans>
                                            Average exceeds{" "}
                                            <strong className="text-foreground">
                                                {value}
                                                {alertData.unit}
                                            </strong>
                                        </Trans>
                                    )}
                                </p>
                                <div className="flex gap-3">
                                    <Slider
                                        aria-labelledby={`v${name}`}
                                        defaultValue={[value]}
                                        onValueCommit={(val) => sendUpsert(min, val[0])}
                                        onValueChange={(val) => setValue(val[0])}
                                        step={alertData.step ?? 1}
                                        min={alertData.min ?? 1}
                                        max={alertData.max ?? 99}
                                    />
                                </div>
                            </div>
                        )}
                        <div className={cn(singleDescription && "col-span-full lowercase")}>
                            <p id={`t${name}`} className="text-sm block h-8 first-letter:uppercase">
                                {singleDescription && (
                                    <>
                                        {singleDescription}
                                        {` `}
                                    </>
                                )}
                                <Trans>
                                    For <strong className="text-foreground">{min}</strong>{" "}
                                    <Plural value={min} one="minute" other="minutes" />
                                </Trans>
                            </p>
                            <div className="flex gap-3">
                                <Slider
                                    aria-labelledby={`v${name}`}
                                    defaultValue={[min]}
                                    onValueCommit={(minVal) => sendUpsert(minVal[0], value)}
                                    onValueChange={(val) => setMin(val[0])}
                                    min={1}
                                    max={60}
                                />
                            </div>
                        </div>
                    </Suspense>
                </div>
            )}
        </div>
    )
}
