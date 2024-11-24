package main

import (
	"beszel"
	"beszel/internal/hub"

	_ "beszel/migrations"

	"github.com/pocketbase/pocketbase"
	"github.com/spf13/cobra"
)

func main() {
	app := pocketbase.NewWithConfig(pocketbase.Config{
		DefaultDataDir: beszel.AppName + "_data",
	})
	app.RootCmd.Version = beszel.Version
	app.RootCmd.Use = beszel.AppName
	app.RootCmd.Short = ""

	// add update command
	app.RootCmd.AddCommand(&cobra.Command{
		Use:   "update",
		Short: "Update " + beszel.AppName + " to the latest version",
		Run:   hub.Update,
	})

	hub.NewHub(app).Run()
}
