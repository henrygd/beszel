package main

import (
	"beszel"
	"beszel/internal/agent"
	"beszel/internal/update"
	"fmt"
	"log"
	"os"
	"strings"
)

func main() {
	// handle flags / subcommands
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "-v":
			fmt.Println(beszel.AppName+"-agent", beszel.Version)
		case "update":
			update.UpdateBeszelAgent()
		}
		os.Exit(0)
	}

	var pubKey []byte
	if pubKeyEnv, exists := os.LookupEnv("KEY"); exists {
		pubKey = []byte(pubKeyEnv)
	} else {
		log.Fatal("KEY environment variable is not set")
	}

	addr := ":45876"
	if portEnvVar, exists := os.LookupEnv("PORT"); exists {
		// allow passing an address in the form of "127.0.0.1:45876"
		if !strings.Contains(portEnvVar, ":") {
			portEnvVar = ":" + portEnvVar
		}
		addr = portEnvVar
	}

	agent.NewAgent().Run(pubKey, addr)
}
