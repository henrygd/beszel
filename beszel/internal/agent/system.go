package agent

import (
	"beszel"
	"beszel/internal/entities/system"
	"bufio"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/host"
	"github.com/shirou/gopsutil/v4/mem"
)

// cgroupInfo stores cgroup-related information
type cgroupInfo struct {
	memoryPath   string
	cpuPath      string
	cpusetPath   string
	assignedCPUs string
	numCPUs      int
	prevCPUUsage uint64
	prevCPUTime  time.Time
}

// getCgroupPath returns the cgroup path for a given subsystem
func getCgroupPath(subsystem string) (string, error) {
	pid := os.Getpid()
	cgroupFile := fmt.Sprintf("/proc/%d/cgroup", pid)
	
	file, err := os.Open(cgroupFile)
	if err != nil {
		return "", fmt.Errorf("failed to open cgroup file: %v", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		fields := strings.Split(scanner.Text(), ":")
		if len(fields) == 3 {
			controllerFields := strings.Split(fields[1], ",")
			for _, controller := range controllerFields {
				if controller == subsystem {
					return fields[2], nil
				}
			}
		}
	}

	return "", fmt.Errorf("no cgroup path found for subsystem: %s", subsystem)
}

// readCgroupFile reads and returns the contents of a cgroup file
func readCgroupFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		slog.Debug("Failed to read cgroup file", "path", path, "error", err)
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}

// initializeCgroupInfo initializes cgroup monitoring
func (a *Agent) initializeCgroupInfo() error {
	a.cgroup = &cgroupInfo{}

	// Get memory cgroup path
	memPath, err := getCgroupPath("memory")
	if err != nil {
		return fmt.Errorf("failed to get memory cgroup path: %v", err)
	}
	a.cgroup.memoryPath = memPath

	// Get CPU cgroup path
	cpuPath, err := getCgroupPath("cpuacct")
	if err != nil {
		return fmt.Errorf("failed to get CPU cgroup path: %v", err)
	}
	a.cgroup.cpuPath = cpuPath

	// Get CPU assignment
	cpusetPath, err := getCgroupPath("cpuset")
	if err == nil {
		a.cgroup.cpusetPath = cpusetPath
		if cpus, err := a.getCPUAssignment(); err == nil {
			a.cgroup.assignedCPUs = cpus
			a.cgroup.numCPUs = countCPUsFromRange(cpus)
			slog.Info("Cgroup CPU assignment", "cpus", cpus, "count", a.cgroup.numCPUs)
		}
	}

	return nil
}

// getCPUAssignment gets the assigned CPUs from cgroup
func (a *Agent) getCPUAssignment() (string, error) {
	cpusPath := filepath.Join("/sys/fs/cgroup/cpuset", a.cgroup.cpusetPath, "cpuset.cpus")
	return readCgroupFile(cpusPath)
}

// countCPUsFromRange counts the number of CPUs from a range string
func countCPUsFromRange(cpuRange string) int {
	count := 0
	ranges := strings.Split(cpuRange, ",")
	for _, r := range ranges {
		bounds := strings.Split(r, "-")
		if len(bounds) == 1 {
			count++
		} else if len(bounds) == 2 {
			start, err1 := strconv.Atoi(bounds[0])
			end, err2 := strconv.Atoi(bounds[1])
			if err1 == nil && err2 == nil {
				count += end - start + 1
			}
		}
	}
	return count
}

// getMemoryFromCgroup gets memory usage from cgroup
func (a *Agent) getMemoryFromCgroup() (uint64, uint64, error) {
	// Get step path (remove task_0 if present)
	stepPath := strings.TrimSuffix(a.cgroup.memoryPath, "/task_0")
	
	// Read memory limit from step level
	stepBasePath := filepath.Join("/sys/fs/cgroup/memory", stepPath)
	limitPath := filepath.Join(stepBasePath, "memory.limit_in_bytes")
	limitStr, err := readCgroupFile(limitPath)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to read memory limit: %v", err)
	}

	memLimit, err := strconv.ParseUint(limitStr, 10, 64)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to parse memory limit: %v", err)
	}

	// Read memory usage from task level
	taskBasePath := filepath.Join("/sys/fs/cgroup/memory", a.cgroup.memoryPath)
	usagePath := filepath.Join(taskBasePath, "memory.usage_in_bytes")
	usageStr, err := readCgroupFile(usagePath)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to read memory usage: %v", err)
	}

	memUsage, err := strconv.ParseUint(usageStr, 10, 64)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to parse memory usage: %v", err)
	}

	// Validate memory limit
	if memLimit < 1024 || memLimit == ^uint64(0) {
		return 0, 0, fmt.Errorf("invalid memory limit: %d", memLimit)
	}

	return memUsage, memLimit, nil
}

// getCPUFromCgroup gets CPU usage from cgroup
func (a *Agent) getCPUFromCgroup() (float64, error) {
	cpuPath := filepath.Join("/sys/fs/cgroup/cpu,cpuacct", a.cgroup.cpuPath, "cpuacct.usage")
	usageStr, err := readCgroupFile(cpuPath)
	if err != nil {
		return 0, fmt.Errorf("failed to read CPU usage: %v", err)
	}

	usageNano, err := strconv.ParseUint(usageStr, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("failed to parse CPU usage: %v", err)
	}

	currentTime := time.Now()
	cpuPerc := 0.0

	if !a.cgroup.prevCPUTime.IsZero() {
		timeDelta := currentTime.Sub(a.cgroup.prevCPUTime).Seconds()
		if timeDelta > 0 {
			cpuDelta := float64(usageNano - a.cgroup.prevCPUUsage)
			if a.cgroup.numCPUs > 0 {
				cpuPerc = (cpuDelta / timeDelta) / (1000000000.0 * float64(a.cgroup.numCPUs)) * 100.0
			}
		}
	}

	a.cgroup.prevCPUUsage = usageNano
	a.cgroup.prevCPUTime = currentTime

	return cpuPerc, nil
}

// Sets initial / non-changing values about the host system
func (a *Agent) initializeSystemInfo() {
	a.systemInfo.AgentVersion = beszel.Version
	a.systemInfo.Hostname, _ = os.Hostname()
	a.systemInfo.KernelVersion, _ = host.KernelVersion()

	// Initialize cgroup monitoring
	if err := a.initializeCgroupInfo(); err != nil {
		slog.Warn("Failed to initialize cgroup monitoring", "error", err)
	}

	// cpu model
	if info, err := cpu.Info(); err == nil && len(info) > 0 {
		a.systemInfo.CpuModel = info[0].ModelName
	}
	
	// If we have cgroup info, use that for cores/threads
	if a.cgroup != nil && a.cgroup.numCPUs > 0 {
		a.systemInfo.Cores = a.cgroup.numCPUs
		a.systemInfo.Threads = a.cgroup.numCPUs
	} else {
		// Fallback to system-wide CPU counts
		a.systemInfo.Cores, _ = cpu.Counts(false)
		if threads, err := cpu.Counts(true); err == nil {
			if threads > 0 && threads < a.systemInfo.Cores {
				a.systemInfo.Cores = threads
			} else {
				a.systemInfo.Threads = threads
			}
		}
	}

	// zfs
	if _, err := getARCSize(); err == nil {
		a.zfs = true
	} else {
		slog.Debug("Not monitoring ZFS ARC", "err", err)
	}
}

// Returns current info, stats about the host system
func (a *Agent) getSystemStats() system.Stats {
	systemStats := system.Stats{}

	// Try to get CPU and memory stats from cgroup first
	if a.cgroup != nil {
		// Get CPU percent from cgroup
		if cpuPct, err := a.getCPUFromCgroup(); err == nil {
			systemStats.Cpu = twoDecimals(cpuPct)
		} else {
			slog.Debug("Failed to get CPU stats from cgroup", "error", err)
			// Fallback to system CPU stats
			if cpuPct, err := cpu.Percent(0, false); err == nil && len(cpuPct) > 0 {
				systemStats.Cpu = twoDecimals(cpuPct[0])
			}
		}

		// Get memory stats from cgroup
		if memUsage, memLimit, err := a.getMemoryFromCgroup(); err == nil {
			systemStats.Mem = bytesToGigabytes(memLimit)
			systemStats.MemUsed = bytesToGigabytes(memUsage)
			systemStats.MemPct = twoDecimals(float64(memUsage) / float64(memLimit) * 100)
		} else {
			slog.Debug("Failed to get memory stats from cgroup", "error", err)
			// Fallback to system memory stats
			if v, err := mem.VirtualMemory(); err == nil {
				systemStats.Mem = bytesToGigabytes(v.Total)
				systemStats.MemUsed = bytesToGigabytes(v.Used)
				systemStats.MemPct = twoDecimals(v.UsedPercent)
			}
		}
	} else {
		// Use system-wide stats if no cgroup info
		if cpuPct, err := cpu.Percent(0, false); err == nil && len(cpuPct) > 0 {
			systemStats.Cpu = twoDecimals(cpuPct[0])
		}
		if v, err := mem.VirtualMemory(); err == nil {
			systemStats.Mem = bytesToGigabytes(v.Total)
			systemStats.MemUsed = bytesToGigabytes(v.Used)
			systemStats.MemPct = twoDecimals(v.UsedPercent)
		}
	}

	return systemStats
}

// Returns the size of the ZFS ARC memory cache in bytes
func getARCSize() (uint64, error) {
	file, err := os.Open("/proc/spl/kstat/zfs/arcstats")
	if err != nil {
		return 0, err
	}
	defer file.Close()

	// Scan the lines
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "size") {
			// Example line: size 4 15032385536
			fields := strings.Fields(line)
			if len(fields) < 3 {
				return 0, err
			}
			// Return the size as uint64
			return strconv.ParseUint(fields[2], 10, 64)
		}
	}

	return 0, fmt.Errorf("failed to parse size field")
}
