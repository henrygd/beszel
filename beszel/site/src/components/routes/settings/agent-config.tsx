"use client"

import { useState, useEffect } from "react"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Button } from "@/components/ui/button"
import { Badge } from "@/components/ui/badge"
import { Separator } from "@/components/ui/separator"
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs"
import { Checkbox } from "@/components/ui/checkbox"
import { Alert, AlertDescription } from "@/components/ui/alert"
import { Label } from "@/components/ui/label"
import { toast } from "@/components/ui/use-toast"
import { Loader2, Save, RefreshCw, AlertTriangle, CheckCircle, XCircle } from "lucide-react"
import { pb } from "@/lib/stores"
import { isAdmin } from "@/lib/utils"
import { redirectPage } from "@nanostores/router"
import { $router } from "@/components/router"
import { Trans } from "@lingui/react/macro"

interface System {
  id: string
  name: string
  host: string
  status: string
  agent_config?: string
}

interface AgentConfig {
  log_level?: string
  mem_calc?: string
  extra_fs?: string[]
  data_dir?: string
  docker_host?: string
  filesystem?: string
  nics?: string
  primary_sensor?: string
  sensors?: string
  sys_sensors?: string
  environment?: Record<string, string>
}

export default function AgentConfig() {
  const [systems, setSystems] = useState<System[]>([])
  const [selectedSystems, setSelectedSystems] = useState<Set<string>>(new Set())
  const [configs, setConfigs] = useState<Record<string, AgentConfig>>({})
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const [selectAll, setSelectAll] = useState(false)

  // Config state
  const [agentConfig, setAgentConfig] = useState<AgentConfig>({
    log_level: "info",
    mem_calc: "",
    extra_fs: [],
    data_dir: "",
    docker_host: "",
    filesystem: "",
    nics: "",
    primary_sensor: "",
    sensors: "",
    sys_sensors: "",
    environment: {}
  })

  // Load systems and their configurations
  useEffect(() => {
    loadSystems()
  }, [])

  const loadSystems = async () => {
    try {
      setLoading(true)
      
      // Load all systems
      const systemsResponse = await pb.collection('systems').getList(1, 1000)
      const systemsData = systemsResponse.items as unknown as System[]
      setSystems(systemsData)

      // Load configurations for each system
      const configsData: Record<string, AgentConfig> = {}
      for (const system of systemsData) {
        try {
          if (system.agent_config) {
            let parsedConfig = {}
            if (typeof system.agent_config === 'object') {
              parsedConfig = system.agent_config
            } else {
              try {
                parsedConfig = JSON.parse(system.agent_config)
              } catch (parseError) {
                console.error('Failed to parse agent_config:', parseError)
                parsedConfig = {}
              }
            }
            configsData[system.id] = parsedConfig as AgentConfig
          }
        } catch (error) {
          console.log(`No config found for system ${system.name} - will create one when needed`)
        }
      }
      setConfigs(configsData)
    } catch (error) {
      console.error('Error loading systems:', error)
      toast({
        title: "Error",
        description: "Failed to load systems",
        variant: "destructive",
      })
    } finally {
      setLoading(false)
    }
  }

  const handleSelectAll = (checked: boolean) => {
    setSelectAll(checked)
    if (checked) {
      setSelectedSystems(new Set(systems.map(s => s.id)))
    } else {
      setSelectedSystems(new Set())
    }
  }

  const handleSelectSystem = (systemId: string, checked: boolean) => {
    const newSelected = new Set(selectedSystems)
    if (checked) {
      newSelected.add(systemId)
    } else {
      newSelected.delete(systemId)
    }
    setSelectedSystems(newSelected)
    setSelectAll(newSelected.size === systems.length)
  }

  const loadConfigFromSelected = () => {
    if (selectedSystems.size === 0) {
      toast({
        title: "Error",
        description: "Please select at least one system",
        variant: "destructive",
      })
      return
    }

    const selectedConfigs = Array.from(selectedSystems)
      .map(id => configs[id])
      .filter(config => config)

    if (selectedConfigs.length === 0) {
      toast({
        title: "Error",
        description: "No configurations found for selected systems",
        variant: "destructive",
      })
      return
    }

    // Use the first config as a template
    const templateConfig = selectedConfigs[0]
    setAgentConfig(templateConfig)

    toast({
      title: "Success",
      description: `Loaded configuration from ${selectedConfigs.length} system(s)`,
    })
  }

  const saveBulkConfig = async () => {
    if (selectedSystems.size === 0) {
      toast({
        title: "Error",
        description: "Please select at least one system",
        variant: "destructive",
      })
      return
    }

    try {
      setSaving(true)
      
      for (const systemId of selectedSystems) {
        const system = systems.find(s => s.id === systemId)
        if (!system) continue

        // Clean up the config before saving
        const cleanConfig = { ...agentConfig }
        
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

        await pb.collection("systems").update(systemId, {
          agent_config: JSON.stringify(cleanConfig)
        })
      }

      toast({
        title: "Success",
        description: `Configuration updated for ${selectedSystems.size} system(s)`,
      })
      await loadSystems() // Reload to get updated data
    } catch (error) {
      console.error('Error saving bulk config:', error)
      toast({
        title: "Error",
        description: "Failed to save configuration",
        variant: "destructive",
      })
    } finally {
      setSaving(false)
    }
  }

  const getSystemStatusIcon = (status: string) => {
    switch (status) {
      case 'up':
        return <CheckCircle className="h-4 w-4 text-green-500" />
      case 'down':
        return <XCircle className="h-4 w-4 text-red-500" />
      default:
        return <AlertTriangle className="h-4 w-4 text-yellow-500" />
    }
  }

  const updateConfig = (key: string, value: any) => {
    setAgentConfig(prev => ({
      ...prev,
      [key]: value
    }))
  }

  const updateEnvironment = (key: string, value: string) => {
    setAgentConfig(prev => ({
      ...prev,
      environment: {
        ...prev.environment,
        [key]: value
      }
    }))
  }

  const removeEnvironment = (key: string) => {
    setAgentConfig(prev => {
      const newEnv = { ...prev.environment }
      delete newEnv[key]
      return {
        ...prev,
        environment: newEnv
      }
    })
  }

  const addEnvironment = () => {
    setAgentConfig(prev => ({
      ...prev,
      environment: {
        ...prev.environment,
        "": ""
      }
    }))
  }

  if (!isAdmin()) {
    redirectPage($router, "settings", { name: "general" })
  }

  if (loading) {
    return (
      <div className="flex items-center justify-center h-64">
        <Loader2 className="h-8 w-8 animate-spin" />
      </div>
    )
  }

  return (
    <div className="space-y-6">
      <div>
        <h2 className="text-2xl font-bold tracking-tight">Bulk Agent Configuration</h2>
        <p className="text-muted-foreground">
          Configure agent settings for multiple systems at once.
        </p>
      </div>

      <div className="grid grid-cols-1 lg:grid-cols-3 gap-6">
        {/* Systems Selection */}
        <Card className="lg:col-span-1">
          <CardHeader>
            <CardTitle>Systems</CardTitle>
            <CardDescription>
              Select systems to configure
            </CardDescription>
          </CardHeader>
          <CardContent className="space-y-4">
            <div className="flex items-center space-x-2">
              <Checkbox
                id="select-all"
                checked={selectAll}
                onCheckedChange={handleSelectAll}
              />
              <Label htmlFor="select-all">Select All ({systems.length})</Label>
            </div>
            
            <Separator />
            
            <div className="space-y-2 max-h-96 overflow-y-auto">
              {systems.map((system) => (
                <div key={system.id} className="flex items-center space-x-2">
                  <Checkbox
                    id={system.id}
                    checked={selectedSystems.has(system.id)}
                    onCheckedChange={(checked) => handleSelectSystem(system.id, checked as boolean)}
                  />
                  <div className="flex-1 min-w-0">
                    <Label htmlFor={system.id} className="text-sm font-medium truncate">
                      {system.name}
                    </Label>
                    <div className="flex items-center space-x-2 text-xs text-muted-foreground">
                      {getSystemStatusIcon(system.status)}
                      <span>{system.host}</span>
                      <Badge variant={system.status === 'up' ? 'default' : 'secondary'}>
                        {system.status}
                      </Badge>
                    </div>
                  </div>
                </div>
              ))}
            </div>

            {selectedSystems.size > 0 && (
              <Alert>
                <AlertDescription>
                  {selectedSystems.size} system(s) selected
                </AlertDescription>
              </Alert>
            )}
          </CardContent>
        </Card>

        {/* Configuration */}
        <Card className="lg:col-span-2">
          <CardHeader>
            <CardTitle>Configuration</CardTitle>
            <CardDescription>
              Configure agent settings for selected systems
            </CardDescription>
          </CardHeader>
          <CardContent>
            {selectedSystems.size === 0 ? (
              <div className="text-center py-8 text-muted-foreground">
                Select systems to configure their settings
              </div>
            ) : (
              <div className="space-y-6">
                {/* Action Buttons */}
                <div className="flex justify-between items-center">
                  <div className="flex space-x-2">
                    <Button
                      variant="outline"
                      onClick={loadConfigFromSelected}
                      disabled={selectedSystems.size === 0}
                    >
                      Load from Selected
                    </Button>
                    <Button
                      variant="outline"
                      onClick={loadSystems}
                      disabled={saving}
                    >
                      <RefreshCw className="h-4 w-4 mr-2" />
                      Refresh
                    </Button>
                  </div>
                  <Button
                    onClick={saveBulkConfig}
                    disabled={saving || selectedSystems.size === 0}
                  >
                    {saving ? (
                      <Loader2 className="h-4 w-4 mr-2 animate-spin" />
                    ) : (
                      <Save className="h-4 w-4 mr-2" />
                    )}
                    Save to {selectedSystems.size} System(s)
                  </Button>
                </div>

                <Separator />

                {/* Agent Configuration Tabs */}
                <Tabs defaultValue="basic" className="w-full">
                  <TabsList className="grid w-full grid-cols-3">
                    <TabsTrigger value="basic">Basic Settings</TabsTrigger>
                    <TabsTrigger value="advanced">Advanced</TabsTrigger>
                    <TabsTrigger value="environment">Environment</TabsTrigger>
                  </TabsList>
                  
                  <TabsContent value="basic" className="mt-6 space-y-4">
                    <div className="space-y-2">
                      <Label htmlFor="log_level">Log Level</Label>
                      <select 
                        id="log_level"
                        value={agentConfig.log_level || "info"} 
                        onChange={(e) => updateConfig("log_level", e.target.value)}
                        className="w-full p-2 border rounded-md"
                      >
                        <option value="debug">Debug</option>
                        <option value="info">Info</option>
                        <option value="warn">Warn</option>
                        <option value="error">Error</option>
                      </select>
                    </div>

                    <div className="space-y-2">
                      <Label htmlFor="data_dir">Data Directory</Label>
                      <input
                        id="data_dir"
                        type="text"
                        value={agentConfig.data_dir || ""}
                        onChange={(e) => updateConfig("data_dir", e.target.value)}
                        placeholder="e.g., /home/user/.config/beszel"
                        className="w-full p-2 border rounded-md"
                      />
                    </div>

                    <div className="space-y-2">
                      <Label htmlFor="docker_host">Docker Host</Label>
                      <input
                        id="docker_host"
                        type="text"
                        value={agentConfig.docker_host || ""}
                        onChange={(e) => updateConfig("docker_host", e.target.value)}
                        placeholder="e.g., unix:///var/run/docker.sock"
                        className="w-full p-2 border rounded-md"
                      />
                    </div>
                  </TabsContent>

                  <TabsContent value="advanced" className="mt-6 space-y-4">
                    <div className="space-y-2">
                      <Label htmlFor="mem_calc">Memory Calculation</Label>
                      <input
                        id="mem_calc"
                        type="text"
                        value={agentConfig.mem_calc || ""}
                        onChange={(e) => updateConfig("mem_calc", e.target.value)}
                        placeholder="e.g., total-available"
                        className="w-full p-2 border rounded-md"
                      />
                    </div>

                    <div className="space-y-2">
                      <Label htmlFor="filesystem">Root Filesystem</Label>
                      <input
                        id="filesystem"
                        type="text"
                        value={agentConfig.filesystem || ""}
                        onChange={(e) => updateConfig("filesystem", e.target.value)}
                        placeholder="e.g., /dev/nvme0n1p2"
                        className="w-full p-2 border rounded-md"
                      />
                    </div>

                    <div className="space-y-2">
                      <Label htmlFor="nics">Network Interfaces</Label>
                      <input
                        id="nics"
                        type="text"
                        value={agentConfig.nics || ""}
                        onChange={(e) => updateConfig("nics", e.target.value)}
                        placeholder="e.g., eth0,wlan0"
                        className="w-full p-2 border rounded-md"
                      />
                    </div>

                    <div className="space-y-2">
                      <Label htmlFor="primary_sensor">Primary Temperature Sensor</Label>
                      <input
                        id="primary_sensor"
                        type="text"
                        value={agentConfig.primary_sensor || ""}
                        onChange={(e) => updateConfig("primary_sensor", e.target.value)}
                        placeholder="e.g., cpu_thermal"
                        className="w-full p-2 border rounded-md"
                      />
                    </div>

                    <div className="space-y-2">
                      <Label htmlFor="sensors">Temperature Sensors</Label>
                      <input
                        id="sensors"
                        type="text"
                        value={agentConfig.sensors || ""}
                        onChange={(e) => updateConfig("sensors", e.target.value)}
                        placeholder="e.g., cpu_thermal,nvme_composite"
                        className="w-full p-2 border rounded-md"
                      />
                    </div>

                    <div className="space-y-2">
                      <Label htmlFor="sys_sensors">Sensors Sys Path</Label>
                      <input
                        id="sys_sensors"
                        type="text"
                        value={agentConfig.sys_sensors || ""}
                        onChange={(e) => updateConfig("sys_sensors", e.target.value)}
                        placeholder="e.g., /sys/class/hwmon"
                        className="w-full p-2 border rounded-md"
                      />
                    </div>

                    <div className="space-y-2">
                      <Label htmlFor="extra_fs">Extra Filesystems</Label>
                      <textarea
                        id="extra_fs"
                        value={agentConfig.extra_fs?.join("\n") || ""}
                        onChange={(e) => updateConfig("extra_fs", e.target.value.split("\n").filter(fs => fs.trim()))}
                        placeholder="Enter filesystem names, one per line"
                        rows={3}
                        className="w-full p-2 border rounded-md resize-none"
                      />
                    </div>
                  </TabsContent>

                  <TabsContent value="environment" className="mt-6 space-y-4">
                    <div className="space-y-4">
                      {Object.entries(agentConfig.environment || {}).map(([key, value]) => (
                        <div key={key} className="flex space-x-2">
                          <input
                            type="text"
                            value={key}
                            onChange={(e) => {
                              const newEnv = { ...agentConfig.environment }
                              delete newEnv[key]
                              newEnv[e.target.value] = value
                              updateConfig("environment", newEnv)
                            }}
                            placeholder="Variable name"
                            className="flex-1 p-2 border rounded-md"
                          />
                          <input
                            type="text"
                            value={value}
                            onChange={(e) => updateEnvironment(key, e.target.value)}
                            placeholder="Value"
                            className="flex-1 p-2 border rounded-md"
                          />
                          <Button
                            variant="outline"
                            size="sm"
                            onClick={() => removeEnvironment(key)}
                            className="px-3"
                          >
                            Remove
                          </Button>
                        </div>
                      ))}
                      <Button
                        variant="outline"
                        onClick={addEnvironment}
                        className="w-full"
                      >
                        Add Environment Variable
                      </Button>
                    </div>
                  </TabsContent>
                </Tabs>
              </div>
            )}
          </CardContent>
        </Card>
      </div>
    </div>
  )
}
