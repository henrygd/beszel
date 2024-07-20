package main

import (
	"log"
	"os"
	"strings"

	"github.com/blang/semver"
	"github.com/rhysd/go-github-selfupdate/selfupdate"
	"github.com/spf13/cobra"
)

func updateBeszel(cmd *cobra.Command, args []string) {
	var latest *selfupdate.Release
	var found bool
	var err error
	currentVersion := semver.MustParse(Version)
	log.Println("Beszel", currentVersion)
	log.Println("Checking for updates...")
	latest, found, err = selfupdate.DetectLatest("henrygd/beszel")

	if err != nil {
		log.Fatal("Error checking for updates:", err)
	}

	if !found {
		log.Fatal("No updates found")
		os.Exit(1)
	}

	log.Println("Latest version", "v", latest.Version)

	if latest.Version.LTE(currentVersion) {
		log.Println("You are up to date")
		return
	}

	var binaryPath string
	log.Printf("Updating from %s to %s...", currentVersion, latest.Version)
	binaryPath, err = os.Executable()
	if err != nil {
		log.Fatal("Error getting binary path:", err)
		os.Exit(1)
	}
	err = selfupdate.UpdateTo(latest.AssetURL, binaryPath)
	if err != nil {
		log.Fatal("Please try rerunning with sudo. Error:", err)
		os.Exit(1)
	}
	log.Printf("Successfully updated: %s -> %s\n\n%s", currentVersion, latest.Version, strings.TrimSpace(latest.ReleaseNotes))
}
