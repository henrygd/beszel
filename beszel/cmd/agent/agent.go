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
	isClient := false
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "-v":
			fmt.Println(beszel.AppName+"-agent", beszel.Version)
		case "update":
			agent.Update()
		case "client":
			isClient = true
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

	addr := ":45877"

	envAddr := ""
	addrEnvVar, specifiedByAddr := agent.GetEnv("ADDR")
	// Legacy from when PORT was used
	portEnvVar, specifiedByPort := agent.GetEnv("PORT")

	if specifiedByAddr {
		envAddr = addrEnvVar
	} else if specifiedByPort {
		envAddr = portEnvVar
	}

	if specifiedByAddr || specifiedByPort {
		if len(envAddr) == 0 && isClient {
			log.Fatal("No address specified for client to connect to, ADDR was empty")
		}

		// allow passing an address in the form of "127.0.0.1:45877"
		if !strings.Contains(envAddr, ":") {
			envAddr = ":" + envAddr
		} else if isClient {
			// set the default port if non is specified for clients
			envAddr = envAddr + ":45877"
		}

		addr = envAddr

	} else if isClient {
		log.Fatal("No address specified for client to connect to (use ADDR env)")
	}

	if isClient {
		_, exists := agent.GetEnv("API_KEY")
		if !exists {
			log.Fatal("Started in client mode without API_KEY specified")
		}
	}

	agent.NewAgent(isClient).Run(pubKey, addr)
}
