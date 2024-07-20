package main

import (
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/blang/semver"
	"github.com/rhysd/go-github-selfupdate/selfupdate"
)

func updateBeszel() {
	var latest *selfupdate.Release
	var found bool
	var err error
	currentVersion := semver.MustParse(Version)
	fmt.Println("beszel-agent", currentVersion)
	fmt.Println("Checking for updates...")
	latest, found, err = selfupdate.DetectLatest("henrygd/beszel")

	if err != nil {
		fmt.Println("Error checking for updates:", err)
		os.Exit(1)
	}

	if !found {
		fmt.Println("No updates found")
		os.Exit(0)
	}

	fmt.Println("Latest version", "v", latest.Version)

	if latest.Version.LTE(currentVersion) {
		fmt.Println("You are up to date")
		return
	}

	var binaryPath string
	fmt.Printf("Updating from %s to %s...", currentVersion, latest.Version)
	binaryPath, err = os.Executable()
	if err != nil {
		fmt.Println("Error getting binary path:", err)
		os.Exit(1)
	}
	err = selfupdate.UpdateTo(latest.AssetURL, binaryPath)
	if err != nil {
		fmt.Println("Please try rerunning with sudo. Error:", err)
		os.Exit(1)
	}
	log.Printf("Successfully updated: %s -> %s\n\n%s", currentVersion, latest.Version, strings.TrimSpace(latest.ReleaseNotes))
}
