package main

import (
	"beszel"
	"beszel/internal/agent"
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
			agent.Update()
		}
		os.Exit(0)
	}

	// Try to get the key from the KEY environment variable.
	pubKey := []byte(os.Getenv("KEY"))

	// If KEY is not set, try to read the key from the file specified by KEY_FILE.
	if len(pubKey) == 0 {
		keyFile, exists := os.LookupEnv("KEY_FILE")
		if !exists {
			log.Fatal("Must set KEY or KEY_FILE environment variable")
		}
		var err error
		pubKey, err = os.ReadFile(keyFile)
		if err != nil {
			log.Fatal(err)
		}
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
