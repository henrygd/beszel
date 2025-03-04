package main

import (
	"beszel"
	"beszel/internal/hub"
	_ "beszel/migrations"
	"os"

	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/plugins/migratecmd"
	"github.com/spf13/cobra"
)

func main() {
	baseApp := getBaseApp()
	h := hub.NewHub(baseApp)
	h.BootstrapHub()
	h.Start()
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
	baseApp.RootCmd.AddCommand(&cobra.Command{
		Use:   "update",
		Short: "Update " + beszel.AppName + " to the latest version",
		Run:   hub.Update,
	})

	// enable auto creation of migration files when making collection changes in the Admin UI
	migratecmd.MustRegister(baseApp, baseApp.RootCmd, migratecmd.Config{
		Automigrate: isDev,
		Dir:         "../../migrations",
	})

	return baseApp
}
