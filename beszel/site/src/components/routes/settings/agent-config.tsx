import { t } from "@lingui/core/macro";
import { Trans } from "@lingui/react/macro";
import { isAdmin } from "@/lib/utils"
import { Separator } from "@/components/ui/separator"
import { Button } from "@/components/ui/button"
import { redirectPage } from "@nanostores/router"
import { $router } from "@/components/router"
import { AlertCircleIcon, ServerIcon, LoaderCircleIcon, SaveIcon } from "lucide-react"
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert"
import { pb } from "@/lib/stores"
import { useState, useEffect } from "react"
import { Textarea } from "@/components/ui/textarea"
import { toast } from "@/components/ui/use-toast"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"

interface AgentConfig {
	log_level?: string;
	mem_calc?: string;
	extra_fs?: string[];
	data_dir?: string;
	docker_host?: string;
	filesystem?: string;
	nics?: string;
	primary_sensor?: string;
	sensors?: string;
	sys_sensors?: string;
	environment?: Record<string, string>;
}

export default function AgentConfig() {
	const [systems, setSystems] = useState<any[]>([])
	const [selectedSystem, setSelectedSystem] = useState<string>("")
	const [config, setConfig] = useState<AgentConfig>({})
	const [isLoading, setIsLoading] = useState(false)
	const [isSaving, setIsSaving] = useState(false)

	useEffect(() => {
		fetchSystems()
	}, [])

	useEffect(() => {
		if (selectedSystem) {
			fetchSystemConfig(selectedSystem)
		}
	}, [selectedSystem])

	async function fetchSystems() {
		try {
			setIsLoading(true)
			const records = await pb.collection("systems").getList(1, 50, {
				sort: "name"
			})
			setSystems(records.items)
			if (records.items.length > 0 && !selectedSystem) {
				setSelectedSystem(records.items[0].id)
			}
		} catch (error: any) {
			toast({
				title: t`Error`,
				description: error.message,
				variant: "destructive",
			})
		} finally {
			setIsLoading(false)
		}
	}

	async function fetchSystemConfig(systemId: string) {
		try {
			const system = await pb.collection("systems").getOne(systemId)
			let parsedConfig = {}
			
			if (system.agent_config) {
				// If it's already an object, use it directly
				if (typeof system.agent_config === 'object') {
					parsedConfig = system.agent_config
				} else {
					// If it's a string, try to parse it
					try {
						parsedConfig = JSON.parse(system.agent_config)
					} catch (parseError) {
						console.error('Failed to parse agent_config:', parseError)
						parsedConfig = {}
					}
				}
			}
			
			setConfig(parsedConfig)
		} catch (error: any) {
			toast({
				title: t`Error`,
				description: error.message,
				variant: "destructive",
			})
		}
	}

	async function saveConfig() {
		if (!selectedSystem) return

		try {
			setIsSaving(true)
			
			// Clean up the config before saving
			const cleanConfig = { ...config }
			
			// Remove empty environment variable keys
			if (cleanConfig.environment) {
				const cleanEnv: Record<string, string> = {}
				Object.entries(cleanConfig.environment).forEach(([key, value]) => {
					if (key.trim() !== "") {
						cleanEnv[key.trim()] = value
					}
				})
				cleanConfig.environment = cleanEnv
			}
			
			await pb.collection("systems").update(selectedSystem, {
				agent_config: JSON.stringify(cleanConfig)
			})
			toast({
				title: t`Configuration saved`,
				description: t`Agent configuration has been updated.`,
			})
		} catch (error: any) {
			toast({
				title: t`Error`,
				description: error.message,
				variant: "destructive",
			})
		} finally {
			setIsSaving(false)
		}
	}

	function updateConfig(key: string, value: any) {
		setConfig(prev => ({
			...prev,
			[key]: value
		}))
	}

	if (!isAdmin()) {
		redirectPage($router, "settings", { name: "general" })
	}

	return (
		<div>
			<div>
				<h3 className="text-xl font-medium mb-2">
					<Trans>Agent Configuration</Trans>
				</h3>
				<p className="text-sm text-muted-foreground leading-relaxed">
					<Trans>Configure agent settings that will be pulled by agents at startup.</Trans>
				</p>
			</div>
            <Separator className="my-4" />
                         <Alert>
                 <AlertCircleIcon className="h-4 w-4" />
                 <AlertTitle><Trans>Configuration Distribution</Trans></AlertTitle>
                 <AlertDescription>
                     <Trans>
                         This configuration will be pulled by the agent when it starts up, and only work with the WebSocket connection.
                         The agent will use these settings to configure its behavior.
                         <br />
                         System environment variables will override hub configuration settings.
                     </Trans>
                 </AlertDescription>
             </Alert>
			<Separator className="my-4" />
			
			{isLoading ? (
				<div className="flex items-center justify-center py-8">
					<LoaderCircleIcon className="h-6 w-6 animate-spin" />
				</div>
			) : (
				<div className="space-y-6">
					<Card>
						<CardHeader>
							<CardTitle className="flex items-center gap-2">
								<ServerIcon className="h-5 w-5" />
								<Trans>System Selection</Trans>
							</CardTitle>
							<CardDescription>
								<Trans>Select a system to configure its agent settings.</Trans>
							</CardDescription>
						</CardHeader>
						<CardContent>
							<Select value={selectedSystem} onValueChange={setSelectedSystem}>
								<SelectTrigger>
									<SelectValue placeholder={t`Select a system`} />
								</SelectTrigger>
								<SelectContent>
									{systems.map((system) => (
										<SelectItem key={system.id} value={system.id}>
											{system.name} ({system.host})
										</SelectItem>
									))}
								</SelectContent>
							</Select>
						</CardContent>
					</Card>

					{selectedSystem && (
						<>
							<Card>
								<CardHeader>
									<CardTitle><Trans>Basic Settings</Trans></CardTitle>
									<CardDescription>
										<Trans>Configure basic agent behavior.</Trans>
									</CardDescription>
								</CardHeader>
								<CardContent className="space-y-4">
									<div className="space-y-2">
										<Label htmlFor="log_level"><Trans>Log Level</Trans></Label>
										<Select 
											value={config.log_level || "info"} 
											onValueChange={(value) => updateConfig("log_level", value)}
										>
											<SelectTrigger>
												<SelectValue />
											</SelectTrigger>
											<SelectContent>
												<SelectItem value="debug">Debug</SelectItem>
												<SelectItem value="info">Info</SelectItem>
												<SelectItem value="warn">Warn</SelectItem>
												<SelectItem value="error">Error</SelectItem>
											</SelectContent>
										</Select>
									</div>

									<div className="space-y-2">
										<Label htmlFor="mem_calc"><Trans>Memory Calculation</Trans></Label>
										<Input
											id="mem_calc"
											value={config.mem_calc || ""}
											onChange={(e) => updateConfig("mem_calc", e.target.value)}
											placeholder="e.g., total-available"
										/>
									</div>

									<div className="space-y-2">
										<Label htmlFor="data_dir"><Trans>Data Directory</Trans></Label>
										<Input
											id="data_dir"
											value={config.data_dir || ""}
											onChange={(e) => updateConfig("data_dir", e.target.value)}
											placeholder="e.g., /home/user/.config/beszel"
										/>
									</div>

									<div className="space-y-2">
										<Label htmlFor="docker_host"><Trans>Docker Host</Trans></Label>
										<Input
											id="docker_host"
											value={config.docker_host || ""}
											onChange={(e) => updateConfig("docker_host", e.target.value)}
											placeholder="e.g., unix:///var/run/docker.sock"
										/>
									</div>

									<div className="space-y-2">
										<Label htmlFor="filesystem"><Trans>Root Filesystem</Trans></Label>
										<Input
											id="filesystem"
											value={config.filesystem || ""}
											onChange={(e) => updateConfig("filesystem", e.target.value)}
											placeholder="e.g., /dev/nvme0n1p2"
										/>
									</div>



									<div className="space-y-2">
										<Label htmlFor="nics"><Trans>Network Interfaces</Trans></Label>
										<Input
											id="nics"
											value={config.nics || ""}
											onChange={(e) => updateConfig("nics", e.target.value)}
											placeholder="e.g., eth0,wlan0"
										/>
									</div>

									<div className="space-y-2">
										<Label htmlFor="primary_sensor"><Trans>Primary Temperature Sensor</Trans></Label>
										<Input
											id="primary_sensor"
											value={config.primary_sensor || ""}
											onChange={(e) => updateConfig("primary_sensor", e.target.value)}
											placeholder="e.g., cpu_thermal"
										/>
									</div>

									<div className="space-y-2">
										<Label htmlFor="sensors"><Trans>Temperature Sensors</Trans></Label>
										<Input
											id="sensors"
											value={config.sensors || ""}
											onChange={(e) => updateConfig("sensors", e.target.value)}
											placeholder="e.g., cpu_thermal,nvme_composite"
										/>
									</div>

									<div className="space-y-2">
										<Label htmlFor="sys_sensors"><Trans>Sensors Sys Path</Trans></Label>
										<Input
											id="sys_sensors"
											value={config.sys_sensors || ""}
											onChange={(e) => updateConfig("sys_sensors", e.target.value)}
											placeholder="e.g., /sys/class/hwmon"
										/>
									</div>

									<div className="space-y-2">
										<Label htmlFor="extra_fs"><Trans>Extra Filesystems</Trans></Label>
										<Textarea
											id="extra_fs"
											value={config.extra_fs?.join("\n") || ""}
											onChange={(e) => updateConfig("extra_fs", e.target.value.split("\n").filter(fs => fs.trim()))}
											placeholder="Enter filesystem names, one per line"
											rows={3}
											className="resize-none"
										/>
									</div>
								</CardContent>
							</Card>
							<Button 
								onClick={saveConfig} 
								disabled={isSaving || (config.environment && Object.keys(config.environment).some(key => key.trim() === ""))}
								className="flex items-center gap-2"
							>
								{isSaving ? (
									<LoaderCircleIcon className="h-4 w-4 animate-spin" />
								) : (
									<SaveIcon className="h-4 w-4" />
								)}
								<Trans>Save Configuration</Trans>
							</Button>
						</>
					)}
				</div>
			)}
		</div>
	)
} 