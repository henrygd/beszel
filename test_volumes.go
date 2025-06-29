package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
)

func main() {
	// Test the Docker API endpoint
	resp, err := http.Get("http://localhost/system/df")
	if err != nil {
		log.Fatal("Error connecting to Docker API:", err)
	}
	defer resp.Body.Close()

	var dfResponse struct {
		Volumes []struct {
			Name      string `json:"Name"`
			UsageData struct {
				Size int64 `json:"Size"`
			} `json:"UsageData"`
		} `json:"Volumes"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&dfResponse); err != nil {
		log.Fatal("Error decoding response:", err)
	}

	fmt.Printf("Found %d volumes:\n", len(dfResponse.Volumes))
	for _, vol := range dfResponse.Volumes {
		if vol.UsageData.Size > 0 {
			sizeMB := float64(vol.UsageData.Size) / (1024 * 1024)
			fmt.Printf("- %s: %.2f MB\n", vol.Name, sizeMB)
		}
	}
} 