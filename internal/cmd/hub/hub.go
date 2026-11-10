package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/henrygd/beszel"
	"github.com/henrygd/beszel/internal/ghupdate"
	"github.com/henrygd/beszel/internal/hub"
	_ "github.com/henrygd/beszel/internal/migrations"

	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/plugins/migratecmd"
	"github.com/spf13/cobra"
)

func main() {
	// handle health check first to prevent unneeded execution
	if len(os.Args) > 3 && os.Args[1] == "health" {
		url := os.Args[3]
		if err := checkHealth(url); err != nil {
			log.Fatal(err)
		}
		fmt.Print("ok")
		return
	}

	baseApp := getBaseApp()

	// handle update before starting the app to prevent PocketBase from
	// bootstrapping a data directory relative to the process working directory
	if len(os.Args) > 1 && os.Args[1] == "update" {
		if err := baseApp.RootCmd.Execute(); err != nil {
			log.Fatal(err)
		}
		return
	}

	hub := hub.NewHub(baseApp)
	if err := hub.StartHub(); err != nil {
		log.Fatal(err)
	}
}

// getBaseApp creates a new PocketBase app with the default config
func getBaseApp() *pocketbase.PocketBase {
	isDev := os.Getenv("ENV") == "dev"

	baseApp := pocketbase.NewWithConfig(pocketbase.Config{
		DefaultDataDir: beszel.AppName + "_data",
		DefaultDev:     isDev,
	})
	baseApp.RootCmd.Version = beszel.Version
	baseApp.RootCmd.Use = beszel.AppName
	baseApp.RootCmd.Short = ""
	// add update command
	baseApp.RootCmd.AddCommand(newUpdateCmd())
	// add health command
	baseApp.RootCmd.AddCommand(newHealthCmd())

	// enable auto creation of migration files when making collection changes in the Admin UI
	migratecmd.MustRegister(baseApp, baseApp.RootCmd, migratecmd.Config{
		Automigrate: isDev,
		Dir:         "../../migrations",
	})

	return baseApp
}

func newUpdateCmd() *cobra.Command {
	updateCmd := &cobra.Command{
		Use:   "update",
		Short: "Update " + beszel.AppName + " to the latest version",
		Run: func(cmd *cobra.Command, _ []string) {
			useMirror, _ := cmd.Flags().GetBool("china-mirrors")
			hub.Update(useMirror)
		},
		Hidden: ghupdate.SelfUpdateDisabled,
	}
	updateCmd.Flags().Bool("china-mirrors", false, "Use mirror (gh.beszel.dev) instead of GitHub")
	return updateCmd
}

func newHealthCmd() *cobra.Command {
	var baseURL string

	healthCmd := &cobra.Command{
		Use:   "health",
		Short: "Check health of running hub",
		Run: func(cmd *cobra.Command, args []string) {
			if err := checkHealth(baseURL); err != nil {
				log.Fatal(err)
			}
			os.Exit(0)
		},
	}
	healthCmd.Flags().StringVar(&baseURL, "url", "", "base URL")
	healthCmd.MarkFlagRequired("url")
	return healthCmd
}

// checkHealth checks the health of the hub.
func checkHealth(baseURL string) error {
	client := &http.Client{
		Timeout: time.Second * 3,
	}
	healthURL := baseURL + "/api/health"
	resp, err := client.Get(healthURL)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("%s returned status %d", healthURL, resp.StatusCode)
	}
	return nil
}
