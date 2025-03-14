package main

import (
	"beszel"
	"beszel/internal/agent"
	"flag"
	"fmt"
	"log"
	"log/slog"
	"os"

	"golang.org/x/crypto/ssh"
)

// cli options
type cmdOptions struct {
	key    string // key is the public key(s) for SSH authentication.
	listen string // listen is the address or port to listen on.
}

// parse parses the command line flags and populates the config struct.
// It returns true if a subcommand was handled and the program should exit.
func (opts *cmdOptions) parse() bool {
	flag.StringVar(&opts.key, "key", "", "Public key(s) for SSH authentication")
	flag.StringVar(&opts.listen, "listen", "", "Address or port to listen on")

	flag.Usage = func() {
		fmt.Printf("Usage: %s [command] [flags]\n", os.Args[0])
		fmt.Println("\nCommands:")
		fmt.Println("  health    Check if the agent is running")
		fmt.Println("  help      Display this help message")
		fmt.Println("  update    Update to the latest version")
		fmt.Println("  version   Display the version")
		fmt.Println("\nFlags:")
		flag.PrintDefaults()
	}

	subcommand := ""
	if len(os.Args) > 1 {
		subcommand = os.Args[1]
	}

	switch subcommand {
	case "-v", "version":
		fmt.Println(beszel.AppName+"-agent", beszel.Version)
		return true
	case "help":
		flag.Usage()
		return true
	case "update":
		agent.Update()
		return true
	case "health":
		// for health, we need to parse flags first to get the listen address
		args := append(os.Args[2:], subcommand)
		flag.CommandLine.Parse(args)
		addr := opts.getAddress()
		network := agent.GetNetwork(addr)
		exitCode, err := agent.Health(addr, network)
		slog.Info("Health", "code", exitCode, "err", err)
		os.Exit(exitCode)
	}

	flag.Parse()
	return false
}

// loadPublicKeys loads the public keys from the command line flag, environment variable, or key file.
func (opts *cmdOptions) loadPublicKeys() ([]ssh.PublicKey, error) {
	// Try command line flag first
	if opts.key != "" {
		return agent.ParseKeys(opts.key)
	}

	// Try environment variable
	if key, ok := agent.GetEnv("KEY"); ok && key != "" {
		return agent.ParseKeys(key)
	}

	// Try key file
	keyFile, ok := agent.GetEnv("KEY_FILE")
	if !ok {
		return nil, fmt.Errorf("no key provided: must set -key flag, KEY env var, or KEY_FILE env var. Use 'beszel-agent help' for usage")
	}

	pubKey, err := os.ReadFile(keyFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read key file: %w", err)
	}
	return agent.ParseKeys(string(pubKey))
}

func (opts *cmdOptions) getAddress() string {
	return agent.GetAddress(opts.listen)
}

func main() {
	var opts cmdOptions
	subcommandHandled := opts.parse()

	if subcommandHandled {
		return
	}

	var serverConfig agent.ServerOptions
	var err error
	serverConfig.Keys, err = opts.loadPublicKeys()
	if err != nil {
		log.Fatal("Failed to load public keys:", err)
	}

	addr := opts.getAddress()
	serverConfig.Addr = addr
	serverConfig.Network = agent.GetNetwork(addr)

	agent := agent.NewAgent()
	if err := agent.StartServer(serverConfig); err != nil {
		log.Fatal("Failed to start server:", err)
	}
}
