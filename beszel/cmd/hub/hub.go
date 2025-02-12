package main

import (
	"beszel/internal/hub"
	_ "beszel/migrations"
)

func main() {
	hub.NewHub().Run()
}
