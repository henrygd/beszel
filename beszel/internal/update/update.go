// Package update handles updating beszel and beszel-agent.
package update

import (
	"beszel"
	"fmt"
	"os"
	"strings"

	"github.com/blang/semver"
	"github.com/rhysd/go-github-selfupdate/selfupdate"
)

func UpdateBeszel() {
	var latest *selfupdate.Release
	var found bool
	var err error
	currentVersion := semver.MustParse(beszel.Version)
	fmt.Println("beszel", currentVersion)
	fmt.Println("Checking for updates...")
	updater, _ := selfupdate.NewUpdater(selfupdate.Config{
		Filters: []string{"beszel_"},
	})
	latest, found, err = updater.DetectLatest("henrygd/beszel")

	if err != nil {
		fmt.Println("Error checking for updates:", err)
		os.Exit(1)
	}

	if !found {
		fmt.Println("No updates found")
		os.Exit(0)
	}

	fmt.Println("Latest version:", latest.Version)

	if latest.Version.LTE(currentVersion) {
		fmt.Println("You are up to date")
		return
	}

	var binaryPath string
	fmt.Printf("Updating from %s to %s...\n", currentVersion, latest.Version)
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
	fmt.Printf("Successfully updated to %s\n\n%s\n", latest.Version, strings.TrimSpace(latest.ReleaseNotes))
}

func UpdateBeszelAgent() {
	var latest *selfupdate.Release
	var found bool
	var err error
	currentVersion := semver.MustParse(beszel.Version)
	fmt.Println("beszel-agent", currentVersion)
	fmt.Println("Checking for updates...")
	updater, _ := selfupdate.NewUpdater(selfupdate.Config{
		Filters: []string{"beszel-agent"},
	})
	latest, found, err = updater.DetectLatest("henrygd/beszel")

	if err != nil {
		fmt.Println("Error checking for updates:", err)
		os.Exit(1)
	}

	if !found {
		fmt.Println("No updates found")
		os.Exit(0)
	}

	fmt.Println("Latest version:", latest.Version)

	if latest.Version.LTE(currentVersion) {
		fmt.Println("You are up to date")
		return
	}

	var binaryPath string
	fmt.Printf("Updating from %s to %s...\n", currentVersion, latest.Version)
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
	fmt.Printf("Successfully updated to %s\n\n%s\n", latest.Version, strings.TrimSpace(latest.ReleaseNotes))
}
