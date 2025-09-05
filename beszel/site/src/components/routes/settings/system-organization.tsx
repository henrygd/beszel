import { useState, useMemo, useEffect, useRef, useCallback } from "react"
import { useStore } from "@nanostores/react"
import { $systems, $userSettings } from "@/lib/stores"
import { pb } from "@/lib/api"
import { SystemRecord } from "@/types"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Card, CardHeader, CardTitle, CardContent } from "@/components/ui/card"
import { GroupInput } from "@/components/ui/group-input"
import { updateSystemList } from "@/lib/utils"
import { InputTags } from "@/components/ui/input-tags"
import { Switch } from "@/components/ui/switch"
import { Label } from "@/components/ui/label"
import { saveSettings } from "./layout"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { AlertCircle, Loader2, SaveIcon, ChevronDown, ChevronRight, ChevronsDownUp } from "lucide-react"
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs"
import TagGroupManager from "@/components/tag-group-manager"

export default function SystemOrganization() {
  const systems = useStore($systems)
  const userSettings = useStore($userSettings)
  // Derive all unique groups from systems - make it reactive
  const allGroups = useMemo(() => 
    Array.from(new Set(systems.map(s => s.group).filter(Boolean))).sort() as string[]
  , [systems])
  const [newGroup, setNewGroup] = useState("")
  const [loading, setLoading] = useState(false)
  const [selectedGroupFilter, setSelectedGroupFilter] = useState<string>("all")
  const [selectedTagFilter, setSelectedTagFilter] = useState<string>("all")
  
  // Track pending changes
  const [pendingChanges, setPendingChanges] = useState<Map<string, { tags?: string[], group?: string }>>(new Map())
  const [isSaving, setIsSaving] = useState(false)
  const [collapsedCards, setCollapsedCards] = useState<Set<string>>(new Set())
  const initialCollapseSet = useRef(false)

  // Optimized tag computation
  const allTags = useMemo(() => {
    const tagSet = new Set<string>()
    for (const sys of systems) {
      if (sys.tags) {
        for (const tag of sys.tags) {
          if (tag) tagSet.add(tag)
        }
      }
    }
    return Array.from(tagSet).sort()
  }, [systems])

  // Optimized system filtering
  const filteredSystems = useMemo(() => {
    if (selectedGroupFilter === "all" && selectedTagFilter === "all") {
      return systems
    }
    return systems.filter(system => {
      const matchesGroup = selectedGroupFilter === "all" || 
        (selectedGroupFilter === "no-group" && !system.group) ||
        system.group === selectedGroupFilter
      const matchesTag = selectedTagFilter === "all" || 
        (system.tags && system.tags.includes(selectedTagFilter))
      return matchesGroup && matchesTag
    })
  }, [systems, selectedGroupFilter, selectedTagFilter])

  // Add group - this is now handled automatically when systems are updated
  const addGroup = () => {
    if (newGroup && !allGroups.includes(newGroup)) {
      setNewGroup("")
    }
  }

  // Optimized pending change handlers with useCallback
  const updatePendingTags = useCallback((system: SystemRecord, tags: string[]) => {
    setPendingChanges(prev => {
      const newMap = new Map(prev)
      const current = newMap.get(system.id) || {}
      newMap.set(system.id, { ...current, tags })
      return newMap
    })
  }, [])

  const updatePendingGroup = useCallback((system: SystemRecord, group: string) => {
    setPendingChanges(prev => {
      const newMap = new Map(prev)
      const current = newMap.get(system.id) || {}
      newMap.set(system.id, { ...current, group })
      return newMap
    })
  }, [])

  // Optimized batch save with concurrent updates
  const saveChanges = useCallback(async () => {
    if (pendingChanges.size === 0) return
    
    setIsSaving(true)
    try {
      // Batch all updates into a single Promise.all for better performance
      const updatePromises: Promise<any>[] = []
      
      for (const [systemId, changes] of pendingChanges.entries()) {
        const updates: Record<string, any> = {}
        if (changes.tags !== undefined) updates.tags = changes.tags
        if (changes.group !== undefined) updates.group = changes.group
        
        if (Object.keys(updates).length > 0) {
          updatePromises.push(pb.collection("systems").update(systemId, updates))
        }
      }
      
      // Execute all updates concurrently
      if (updatePromises.length > 0) {
        await Promise.all(updatePromises)
        await updateSystemList()
      }
      setPendingChanges(new Map())
    } catch (error) {
      console.error('Failed to save changes:', error)
    } finally {
      setIsSaving(false)
    }
  }, [pendingChanges])

  // Memoized current value getters for better performance
  const getCurrentTags = useCallback((system: SystemRecord) => {
    const pending = pendingChanges.get(system.id)
    return pending?.tags !== undefined ? pending.tags : (system.tags || [])
  }, [pendingChanges])

  const getCurrentGroup = useCallback((system: SystemRecord) => {
    const pending = pendingChanges.get(system.id)
    return pending?.group !== undefined ? pending.group : (system.group || "")
  }, [pendingChanges])

  // Get pending changes description for a system
  const getPendingChangesDescription = (system: SystemRecord) => {
    const pending = pendingChanges.get(system.id)
    if (!pending) return null
    
    const changes = []
    if (pending.tags !== undefined) changes.push('tags')
    if (pending.group !== undefined) changes.push('group')
    
    return changes.join(', ')
  }

  // Remove system from group
  const removeFromGroup = async (system: SystemRecord) => {
    setLoading(true)
    await pb.collection("systems").update(system.id, { group: "" })
    setLoading(false)
  }

  // Assign system to group
  const assignToGroup = async (system: SystemRecord, group: string) => {
    setLoading(true)
    await pb.collection("systems").update(system.id, { group })
    await updateSystemList()
    setLoading(false)
  }

  const updateTags = async (system: SystemRecord, tags: string[]) => {
    setLoading(true)
    await pb.collection("systems").update(system.id, { tags })
    await updateSystemList()
    setLoading(false)
  }

  const allSystemIds = useMemo(() => filteredSystems.map(s => s.id), [filteredSystems])
  const allCollapsed = allSystemIds.length > 0 && allSystemIds.every(id => collapsedCards.has(id))

  const toggleCollapse = (systemId: string) => {
    setCollapsedCards(prev => {
      const next = new Set(prev)
      if (next.has(systemId)) {
        next.delete(systemId)
      } else {
        next.add(systemId)
      }
      return next
    })
  }

  const handleCollapseAll = () => {
    if (allCollapsed) {
      // Expand all
      setCollapsedCards(new Set())
    } else {
      // Collapse all
      setCollapsedCards(new Set(allSystemIds))
    }
  }

  // Collapse all by default if more than 3 systems, only on first mount or when systems list changes
  useEffect(() => {
    if (!initialCollapseSet.current) {
      if (systems.length > 3) {
        setCollapsedCards(new Set(systems.map(s => s.id)))
      } else {
        setCollapsedCards(new Set())
      }
      initialCollapseSet.current = true
    }
  }, [systems])

  return (
    <Card>
      <CardHeader>
        <CardTitle>System Organization</CardTitle>
      </CardHeader>
      <CardContent>
        <Tabs defaultValue="organize" className="w-full">
          <TabsList className="grid w-full grid-cols-2">
            <TabsTrigger value="organize">Organize Systems</TabsTrigger>
            <TabsTrigger value="manage">Manage Tags & Groups</TabsTrigger>
          </TabsList>
          
          <TabsContent value="organize" className="mt-6">
        <div className="flex items-center space-x-2 mb-6 p-4 bg-muted/30 rounded-lg">
          <Switch
            id="grouping-toggle"
            checked={userSettings.groupingEnabled}
            onCheckedChange={(value) => {
              saveSettings({ groupingEnabled: value })
            }}
          />
          <Label htmlFor="grouping-toggle" className="text-sm font-medium">
            Enable grouping on dashboard
          </Label>
          <span className="text-xs text-muted-foreground ml-2">
            When enabled, systems will be grouped by their assigned groups on the main dashboard
          </span>
        </div>

        {/* Filters */}
        <div className="flex flex-col sm:flex-row gap-4 mb-6">
          <div className="flex flex-col gap-2">
            <Label htmlFor="group-filter" className="text-sm font-medium">Filter by Group</Label>
            <Select value={selectedGroupFilter} onValueChange={setSelectedGroupFilter}>
              <SelectTrigger id="group-filter" className="w-full sm:w-48">
                <SelectValue placeholder="All groups" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="all">All Groups</SelectItem>
                {allGroups.filter(Boolean).map(group => (
                  <SelectItem key={group} value={group}>{group}</SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
          
          <div className="flex flex-col gap-2">
            <Label htmlFor="tag-filter" className="text-sm font-medium">Filter by Tag</Label>
            <Select value={selectedTagFilter} onValueChange={setSelectedTagFilter}>
              <SelectTrigger id="tag-filter" className="w-full sm:w-48">
                <SelectValue placeholder="All tags" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="all">All Tags</SelectItem>
                {allTags.filter(Boolean).map(tag => (
                  <SelectItem key={tag} value={tag}>{tag}</SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
        </div>
        
        {/* Collapse/Expand All button */}
        <div className="flex justify-end mb-2">
          <Button variant="outline" size="sm" onClick={handleCollapseAll} disabled={allSystemIds.length === 0} className="gap-1">
            <ChevronsDownUp className="h-4 w-4" />
            {allCollapsed ? 'Expand All' : 'Collapse All'}
          </Button>
        </div>

        <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-4">
          {filteredSystems.map(system => {
            const hasPendingChanges = pendingChanges.has(system.id)
            const isCollapsed = collapsedCards.has(system.id)
            return (
              <Card key={system.id} className={`p-4 flex flex-col gap-2 ${hasPendingChanges ? 'ring-2 ring-blue-200 bg-blue-50/30' : ''}`}>
                <div className="flex items-center justify-between">
                  <div className="font-semibold text-lg mb-1 flex items-center gap-2">
                    <button
                      type="button"
                      className="focus:outline-none"
                      onClick={() => toggleCollapse(system.id)}
                      aria-label={isCollapsed ? 'Expand' : 'Collapse'}
                    >
                      {isCollapsed ? (
                        <ChevronRight className="h-4 w-4" />
                      ) : (
                        <ChevronDown className="h-4 w-4" />
                      )}
                    </button>
                    {system.name}
                  </div>
                  {hasPendingChanges && (
                    <div className="flex items-center gap-1 text-blue-600">
                      <AlertCircle className="h-3 w-3" />
                      <span className="text-xs">
                        {getPendingChangesDescription(system)}
                      </span>
                    </div>
                  )}
                </div>
                {!isCollapsed && (
                  <>
                    <div className="text-muted-foreground text-sm mb-2"><span className="font-semibold">Host / IP:</span> {system.host}</div>
                    <div className="text-xs font-semibold text-muted-foreground mb-1 mt-2">Tags</div>
                    <InputTags 
                      value={getCurrentTags(system)} 
                      onChange={(tags) => updatePendingTags(system, Array.isArray(tags) ? tags : tags(getCurrentTags(system)))} 
                    />
                    <div className="text-xs font-semibold text-muted-foreground mb-1 mt-3">Group</div>
                    <GroupInput
                      value={getCurrentGroup(system)}
                      groups={allGroups}
                      onChange={g => updatePendingGroup(system, g)}
                      disabled={loading}
                    />
                  </>
                )}
              </Card>
            )
          })}
        </div>

        {/* Save button for pending changes - now at the bottom */}
        {pendingChanges.size > 0 && (
          <div className="flex items-center justify-between mt-8 p-4 bg-muted/30 rounded-lg">
            <div className="flex items-center gap-2">
              <AlertCircle className="h-4 w-4 text-muted-foreground" />
              <span className="text-sm text-muted-foreground">
                {pendingChanges.size} change{pendingChanges.size !== 1 ? 's' : ''} pending
              </span>
            </div>
            <Button 
              onClick={saveChanges} 
              disabled={isSaving}
              className="flex items-center gap-1.5 disabled:opacity-100"
            >
              {isSaving ? (
                <>
                  <Loader2 className="h-4 w-4 animate-spin" />
                  Saving...
                </>
              ) : (
                <>
                  <SaveIcon className="h-4 w-4" />
                  Save Changes
                </>
              )}
            </Button>
          </div>
        )}
          </TabsContent>
          
          <TabsContent value="manage" className="mt-6">
            <TagGroupManager />
          </TabsContent>
        </Tabs>
      </CardContent>
    </Card>
  )
} 