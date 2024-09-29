package agent

import "math"

// delete container stats from map using mutex
func (a *Agent) deleteContainerStatsSync(id string) {
	a.containerStatsMutex.Lock()
	defer a.containerStatsMutex.Unlock()
	delete(a.containerStatsMap, id)
}

func bytesToMegabytes(b float64) float64 {
	return twoDecimals(b / 1048576)
}

func bytesToGigabytes(b uint64) float64 {
	return twoDecimals(float64(b) / 1073741824)
}

func twoDecimals(value float64) float64 {
	return math.Round(value*100) / 100
}
