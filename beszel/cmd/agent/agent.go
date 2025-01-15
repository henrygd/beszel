package main

import (
	"beszel"
	"beszel/internal/agent"
	"fmt"
	"log"
	"os"
	"strings"
	"io/ioutil"
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

	var pubKey []byte
	if pubKeyEnv, exists := os.LookupEnv("KEY"); exists {
		pubKey = []byte(pubKeyEnv)
	} else {
		keyFile := os.Getenv("KEY_FILE")
		if keyFile != "" {
			if keyData, err := ioutil.ReadFile(keyFile); err == nil {
				pubKey = keyData
			} else {
				log.Fatalf("Failed to read key from file '%s': %v", keyFile, err)
			}
		} else {
			log.Fatal("KEY environment variable is not set, and KEY_FILE environment variable is not set")
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
