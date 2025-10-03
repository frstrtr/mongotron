package main

import (
	"fmt"
	"os"

	"github.com/spf13/pflag"
)

const (
	version = "1.0.0"
)

func main() {
	var (
		showVersion bool
		command     string
	)

	pflag.BoolVarP(&showVersion, "version", "v", false, "Show version information")
	pflag.Parse()

	if showVersion {
		fmt.Printf("MongoTron CLI v%s\n", version)
		os.Exit(0)
	}

	args := pflag.Args()
	if len(args) == 0 {
		printUsage()
		os.Exit(1)
	}

	command = args[0]

	switch command {
	case "subscribe":
		handleSubscribe(args[1:])
	case "unsubscribe":
		handleUnsubscribe(args[1:])
	case "status":
		handleStatus(args[1:])
	case "list":
		handleList(args[1:])
	default:
		fmt.Printf("Unknown command: %s\n", command)
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println("MongoTron CLI - Blockchain monitoring management tool")
	fmt.Println()
	fmt.Println("Usage:")
	fmt.Println("  mongotron-cli [command] [options]")
	fmt.Println()
	fmt.Println("Available Commands:")
	fmt.Println("  subscribe     Subscribe to address monitoring")
	fmt.Println("  unsubscribe   Unsubscribe from address monitoring")
	fmt.Println("  status        Check subscription status")
	fmt.Println("  list          List all subscriptions")
	fmt.Println()
	fmt.Println("Flags:")
	fmt.Println("  -v, --version   Show version information")
}

func handleSubscribe(args []string) {
	// TODO: Implement subscribe command
	fmt.Println("Subscribe command not yet implemented")
}

func handleUnsubscribe(args []string) {
	// TODO: Implement unsubscribe command
	fmt.Println("Unsubscribe command not yet implemented")
}

func handleStatus(args []string) {
	// TODO: Implement status command
	fmt.Println("Status command not yet implemented")
}

func handleList(args []string) {
	// TODO: Implement list command
	fmt.Println("List command not yet implemented")
}
