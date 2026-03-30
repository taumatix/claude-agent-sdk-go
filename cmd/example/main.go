// Package main demonstrates basic usage of the Claude Agent SDK.
package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/taumatix/claude-agent-sdk-go/domains/agent"
)

func main() {
	prompt := "What is 2+2? Answer briefly."
	if len(os.Args) > 1 {
		prompt = os.Args[1]
	}

	ctx := context.Background()

	opts := agent.DefaultOptions()
	// opts.CLIPath = "/path/to/claude"  // override if needed

	fmt.Printf("Prompt: %s\n\n", prompt)

	for msg, err := range agent.Query(ctx, prompt, opts) {
		if err != nil {
			log.Fatalf("error: %v", err)
		}

		if msg.Assistant != nil {
			for _, block := range msg.Assistant.Content {
				if block.Text != nil {
					fmt.Print(block.Text.Text)
				}
			}
		}

		if msg.Result != nil {
			fmt.Printf("\n\n--- Result ---\n")
			fmt.Printf("Turns: %d\n", msg.Result.NumTurns)
			fmt.Printf("Duration: %dms\n", msg.Result.DurationMS)
			if msg.Result.TotalCostUSD != nil {
				fmt.Printf("Cost: $%.6f\n", *msg.Result.TotalCostUSD)
			}
			if msg.Result.IsError {
				fmt.Println("Session ended with error")
				os.Exit(1)
			}
		}
	}
}
