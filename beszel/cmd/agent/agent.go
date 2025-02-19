package main

import (
	"beszel"
	"beszel/internal/agent"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	"golang.org/x/crypto/ssh"
)

type cmdConfig struct {
	key  string // key is the public key(s) for SSH authentication.
	addr string // addr is the address or port to listen on.
}

// parseFlags parses the command line flags and populates the config struct.
func parseFlags(cfg *cmdConfig) {
	flag.StringVar(&cfg.key, "key", "", "Public key(s) for SSH authentication")
	flag.StringVar(&cfg.addr, "addr", "", "Address or port to listen on")

	flag.Usage = func() {
		fmt.Printf("Usage: %s [options] [subcommand]\n", os.Args[0])
		fmt.Println("\nOptions:")
		flag.PrintDefaults()
		fmt.Println("\nSubcommands:")
		fmt.Println("  version      Display the version")
		fmt.Println("  help         Display this help message")
		fmt.Println("  update       Update the agent to the latest version")
	}
}

// handleSubcommand handles subcommands such as version, help, and update.
// It returns true if a subcommand was handled, false otherwise.
func handleSubcommand() bool {
	if len(os.Args) <= 1 {
		return false
	}
	switch os.Args[1] {
	case "version", "-v":
		fmt.Println(beszel.AppName+"-agent", beszel.Version)
		os.Exit(0)
	case "help":
		flag.Usage()
		os.Exit(0)
	case "update":
		agent.Update()
		os.Exit(0)
	}
	return false
}

// loadPublicKeys loads the public keys from the command line flag, environment variable, or key file.
func loadPublicKeys(cfg cmdConfig) ([]ssh.PublicKey, error) {
	// Try command line flag first
	if cfg.key != "" {
		return agent.ParseKeys(cfg.key)
	}

	// Try environment variable
	if key, ok := agent.GetEnv("KEY"); ok && key != "" {
		return agent.ParseKeys(key)
	}

	// Try key file
	keyFile, ok := agent.GetEnv("KEY_FILE")
	if !ok {
		return nil, fmt.Errorf("no key provided: must set -key flag, KEY env var, or KEY_FILE env var. ")
	}

	pubKey, err := os.ReadFile(keyFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read key file: %w", err)
	}
	return agent.ParseKeys(string(pubKey))
}

// getAddress gets the address to listen on from the command line flag, environment variable, or default value.
func getAddress(addr string) string {
	// Try command line flag first
	if addr != "" {
		return addr
	}
	// Try environment variables
	if addr, ok := agent.GetEnv("ADDR"); ok && addr != "" {
		return addr
	}
	// Legacy PORT environment variable support
	if port, ok := agent.GetEnv("PORT"); ok && port != "" {
		return port
	}
	return ":45876"
}

// getNetwork returns the network type to use for the server.
func getNetwork(addr string) string {
	if network, _ := agent.GetEnv("NETWORK"); network != "" {
		return network
	}
	if strings.HasPrefix(addr, "/") {
		return "unix"
	}
	return "tcp"
}

func main() {
	var cfg cmdConfig
	parseFlags(&cfg)

	if handleSubcommand() {
		return
	}

	flag.Parse()

	var serverConfig agent.ServerConfig
	var err error
	serverConfig.Keys, err = loadPublicKeys(cfg)
	if err != nil {
		log.Fatal("Failed to load public keys:", err)
	}

	serverConfig.Addr = getAddress(cfg.addr)
	serverConfig.Network = getNetwork(cfg.addr)

	agent := agent.NewAgent()
	if err := agent.StartServer(serverConfig); err != nil {
		log.Fatal("Failed to start server:", err)
	}
}
