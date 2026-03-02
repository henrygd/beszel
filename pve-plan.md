# Goal

Add Proxmox VE (PVE) stats for VMs and LXCs to Beszel, working similarly to how Docker containers are tracked: collect every minute, show stacked area graphs on the system page for CPU, mem, network, plus a dedicated global table/page showing all VMs/LXCs with current data.

# Instructions

- Reuse container.Stats struct for PVE data transport and storage (compatible with existing averaging functions after minor refactor)
- Store clean VM name (without type) in container.Stats.Name; store Proxmox resource ID (e.g. "qemu/100") in container.Stats.Id field; store type ("qemu" or "lxc") in container.Stats.Image field — both Id and Image are json:"-" / CBOR-only so they don't pollute stored JSON stats
- Add PROXMOX_INSECURE_TLS env var (default true) for TLS verification control
- Filter to only running VMs/LXCs (check resource.Status == "running")
- pve_vms table row ID: use makeStableHashId(systemId, resourceId) — same FNV-32a hash used for systemd services, returns 8 hex chars
- VM type (qemu/lxc) stripped from name, shown as a separate badge column in the PVE table
- PVE page is global (shows VMs/LXCs across all systems, with a System column) — mirrors the containers page
- pve_stats collection structure is identical to container_stats (same fields: id, system, stats JSON, type select, created, updated)
- pve_vms collection needs: id (8-char hex), system (relation), name (text), type (text: "qemu"/"lxc"), cpu (number), memory (number), net (number), updated (number)
  Discoveries
- container.Stats struct has Id, Image, Status fields tagged json:"-" but transmitted via CBOR (keys 7, 8, 6 respectively) — perfect for carrying PVE-specific metadata without polluting stored JSON. Id is cbor key 7, Image is cbor key 8.
- makeStableHashId(...strings) already exists in internal/hub/systems/system.go using FNV-32a, returns 8-char hex — reuse this for pve_vms row IDs
- AverageContainerStats() in records.go hardcodes "container_stats" table name — needs to be refactored to accept a collectionName parameter to be reused for pve_stats
- deleteOldSystemStats() in records.go hardcodes [2]string{"system_stats", "container_stats"} — needs pve_stats added
- ContainerChart component reads chartData.containerData directly from the chartData prop — it cannot be reused for PVE without modification. For PVE charts, we need to either modify ContainerChart to accept a custom data prop, or create a PveChart wrapper that passes PVE-shaped chartData. The simplest approach: pass a modified chartData object with containerData replaced by pveData to a reused ContainerChart.
- useContainerChartConfigs(containerData) in hooks.ts is pure and can be called with any ChartData["containerData"]-shaped array — perfectly reusable for PVE.
- $containerFilter atom is in internal/site/src/lib/stores.ts — need to add $pveFilter atom similarly
- ChartData interface in types.d.ts needs a pveData field added alongside containerData
- The getStats() function in system.tsx is generic and reusable for fetching pve_stats
- go-proxmox v0.4.0 is already a direct dependency in go.mod
- Migration files are in internal/migrations/ — only 2 files exist currently; new migration should be named sequentially (e.g., 1_pve_collections.go)
- The ContainerChart component reads const { containerData } = chartData on line 37. The fix is to pass a synthetic chartData that has containerData replaced by pveData when rendering PVE charts.
- createContainerRecords() in system.go checks data.Containers[0].Id != "" before saving — PVE stats use .Id to store resource ID (e.g. "qemu/100"), so that check will work correctly for PVE too
- AverageContainerStats in records.go uses stat.Name as the key for sums map — PVE data stored with clean name (no type suffix) works correctly
- The containers table collection ID is "pbc_1864144027" — need unique IDs for new collections in the migration
- The container_stats collection ID is "juohu4jipgc13v7" — need unique IDs for pve_stats and pve_vms
- The containers table id field pattern is [a-f0-9]{6} min 6, max 12 — for pve_vms we want 8-char hex IDs: set pattern [a-f0-9]{8}, min 8, max 8
- In the migration file, use app.ImportCollectionsByMarshaledJSON with false for upsert (same as existing migration)
- CombinedData struct in system.go uses cbor keys 0-4 for existing fields — PVEStats should be cbor key 5
- For pve\*stats in CreateLongerRecords(), need to add it to the collections array and handle it similarly to container_stats
  Accomplished
  Planning and file reading phase complete. No files have been written/modified yet.
  All relevant source files have been read and fully understood. The complete implementation plan is finalized. Ready to begin writing code.
  Files to CREATE

  | File                                                         | Purpose                                                              |
  | ------------------------------------------------------------ | -------------------------------------------------------------------- |
  | internal/migrations/1_pve_collections.go                     | New pve_stats + pve_vms DB collections                               |
  | internal/site/src/components/routes/pve.tsx                  | Global PVE VMs/LXCs page route                                       |
  | internal/site/src/components/pve-table/pve-table.tsx         | PVE table component (mirrors containers-table)                       |
  | internal/site/src/components/pve-table/pve-table-columns.tsx | Table column defs (Name, System, Type badge, CPU, Mem, Net, Updated) |

  Files to MODIFY

  | File | Changes needed |
  |---|---|
  | agent/pve.go | Refactor: clean name (no type suffix), store resourceId in .Id, type in .Image, filter running-only, add PROXMOX*INSECURE_TLS env var, change network tracking to use bytes |
  | agent/agent.go | Add pveManager *pveManager field to Agent struct; initialize in NewAgent(); call getPVEStats() in gatherStats(), store in data.PVEStats |
  | internal/entities/system/system.go | Add PVEStats []*container.Stats \json:"pve,omitempty" cbor:"5,keyasint,omitempty"\` to CombinedData` struct |
  | internal/hub/systems/system.go | In createRecords(): if data.PVEStats non-empty, save pve_stats record + call new createPVEVMRecords() function |
  | internal/records/records.go | Refactor AverageContainerStats(db, records) → AverageContainerStats(db, records, collectionName); add pve_stats to CreateLongerRecords() and deleteOldSystemStats(); add deleteOldPVEVMRecords() called from DeleteOldRecords() |
  | internal/site/src/components/routes/system.tsx | Add pveData state + fetching (reuse getStats() for pve_stats); add usePVEChartConfigs; add 3 PVE ChartCard+ContainerChart blocks (CPU/Mem/Net) using synthetic chartData with pveData as containerData; add PVE filter bar; reset $pveFilter on unmount |
  | internal/site/src/components/router.tsx | Add pve: "/pve" route |
  | internal/site/src/components/navbar.tsx | Add PVE nav link (using ServerIcon or BoxesIcon) between Containers and SMART links |
  | internal/site/src/lib/stores.ts | Add export const $pveFilter = atom("") |
  | internal/site/src/types.d.ts | Add PveStatsRecord interface (same shape as ContainerStatsRecord); add PveVMRecord interface; add pveData to ChartData interface |
  Key Implementation Notes
  agent/pve.go
  // Filter running only: skip if resource.Status != "running"
  // Store type without type in name: resourceStats.Name = resource.Name
  // Store resource ID: resourceStats.Id = resource.ID (e.g. "qemu/100")
  // Store type: resourceStats.Image = resource.Type (e.g. "qemu" or "lxc")
  // PROXMOX_INSECURE_TLS: default true, parse "false" to disable
  insecureTLS := true
  if val, exists := GetEnv("PROXMOX_INSECURE_TLS"); exists {
  insecureTLS = val != "false"
  }
  internal/hub/systems/system.go — createPVEVMRecords
  func createPVEVMRecords(app core.App, data []\*container.Stats, systemId string) error {
  params := dbx.Params{"system": systemId, "updated": time.Now().UTC().UnixMilli()}
  valueStrings := make([]string, 0, len(data))
  for i, vm := range data {
  suffix := fmt.Sprintf("%d", i)
  valueStrings = append(valueStrings, fmt.Sprintf("({:id%[1]s}, {:system}, {:name%[1]s}, {:type%[1]s}, {:cpu%[1]s}, {:memory%[1]s}, {:net%[1]s}, {:updated})", suffix))
  params["id"+suffix] = makeStableHashId(systemId, vm.Id)
  params["name"+suffix] = vm.Name
  params["type"+suffix] = vm.Image // "qemu" or "lxc"
  params["cpu"+suffix] = vm.Cpu
  params["memory"+suffix] = vm.Mem
  netBytes := vm.Bandwidth[0] + vm.Bandwidth[1]
  if netBytes == 0 {
  netBytes = uint64((vm.NetworkSent + vm.NetworkRecv) * 1024 \* 1024)
  }
  params["net"+suffix] = netBytes
  }
  queryString := fmt.Sprintf(
  "INSERT INTO pve*vms (id, system, name, type, cpu, memory, net, updated) VALUES %s ON CONFLICT(id) DO UPDATE SET system=excluded.system, name=excluded.name, type=excluded.type, cpu=excluded.cpu, memory=excluded.memory, net=excluded.net, updated=excluded.updated",
  strings.Join(valueStrings, ","),
  )
  *, err := app.DB().NewQuery(queryString).Bind(params).Execute()
  return err
  }
  system.tsx — PVE chart rendering pattern
  // Pass synthetic chartData to ContainerChart so it reads pveData as containerData
  const pveSyntheticChartData = useMemo(() => ({...chartData, containerData: pveData}), [chartData, pveData])
  // Then:

  <ContainerChart chartData={pveSyntheticChartData} dataKey="c" chartType={ChartType.CPU} chartConfig={pveChartConfigs.cpu} />
  records.go — AverageContainerStats refactor
  // Change signature to accept collectionName:
  func (rm \*RecordManager) AverageContainerStats(db dbx.Builder, records RecordIds, collectionName string) []container.Stats
  // Change hardcoded query string:
  db.NewQuery(fmt.Sprintf("SELECT stats FROM %s WHERE id = {:id}", collectionName)).Bind(queryParams).One(&statsRecord)
  // In CreateLongerRecords, update all callers:
  longerRecord.Set("stats", rm.AverageContainerStats(db, recordIds, collection.Name))
  Relevant files / directories
  agent/pve.go # Main file to refactor
  agent/agent.go # Add pveManager field + initialization
  internal/entities/system/system.go # Add PVEStats to CombinedData
  internal/entities/container/container.go # container.Stats struct (reused for PVE)
  internal/hub/systems/system.go # Hub record creation (createRecords, makeStableHashId)
  internal/records/records.go # Longer records + deletion
  internal/migrations/0_collections_snapshot_0_18_0_dev_2.go # Reference for collection schema format
  internal/migrations/1_pve_collections.go # TO CREATE
  internal/site/src/ # Frontend root
  internal/site/src/types.d.ts # TypeScript types
  internal/site/src/lib/stores.ts # Nanostores atoms
  internal/site/src/components/router.tsx # Routes
  internal/site/src/components/navbar.tsx # Navigation
  internal/site/src/components/routes/system.tsx # System detail page (1074 lines) - add PVE charts
  internal/site/src/components/routes/containers.tsx # Reference for pve.tsx structure
  internal/site/src/components/routes/pve.tsx # TO CREATE
  internal/site/src/components/charts/container-chart.tsx # Reused for PVE charts (reads chartData.containerData)
  internal/site/src/components/charts/hooks.ts # useContainerChartConfigs (reused for PVE)
  internal/site/src/components/containers-table/containers-table.tsx # Reference for PVE table
  internal/site/src/components/containers-table/containers-table-columns.tsx # Reference for PVE columns
  internal/site/src/components/pve-table/ # TO CREATE (directory + 2 files)
