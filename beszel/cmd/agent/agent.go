package main

import (
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/henrygd/beszel"
	"github.com/henrygd/beszel/internal/agent"
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
	key, _ := agent.GetEnv("KEY")
	pubKey := []byte(key)

	// If KEY is not set, try to read the key from the file specified by KEY_FILE.
	if len(pubKey) == 0 {
		keyFile, exists := agent.GetEnv("KEY_FILE")
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
	// TODO: change env var to ADDR
	if portEnvVar, exists := agent.GetEnv("PORT"); exists {
		// allow passing an address in the form of "127.0.0.1:45876"
		if !strings.Contains(portEnvVar, ":") {
			portEnvVar = ":" + portEnvVar
		}
		addr = portEnvVar
	}

	agent.NewAgent().Run(pubKey, addr)
}
