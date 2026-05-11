package main

import (
	"bufio"
	"cms-chat/chat"
	"cms-chat/client"
	"fmt"
	"os"
	"strings"
)

const (
	colorReset = "\033[0m"
	colorBold  = "\033[1m"
	colorGreen = "\033[32m"
	colorRed   = "\033[31m"
	colorCyan  = "\033[36m"
)

func loadEnv(filename string) {
	f, err := os.Open(filename)
	if err != nil {
		return
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		val := strings.TrimSpace(parts[1])
		if os.Getenv(key) == "" {
			os.Setenv(key, val)
		}
	}
}

func main() {
	loadEnv(".env")

	cmsToken  := mustEnv("CMS_TOKEN")
	workspace := mustEnv("CMS_WORKSPACE")
	project   := mustEnv("CMS_PROJECT")
	model     := mustEnv("CMS_MODEL")
	groqKey   := os.Getenv("GROQ_API_KEY") // optional

	cmsClient   := client.New(cmsToken, workspace, project, model)
	chatHandler := chat.New(cmsClient, groqKey)

	fmt.Printf("\n%s%s Re:Earth CMS Chat%s\n", colorBold, colorCyan, colorReset)
	fmt.Printf("%s\n\n", strings.Repeat("─", 40))
	fmt.Printf("Config:  %s\n", cmsClient.Info())
	if groqKey != "" {
		fmt.Printf("AI:      %sGroq%s (llama-3.3-70b-versatile)\n", colorGreen, colorReset)
	} else {
		fmt.Printf("AI:      keyword matching (set GROQ_API_KEY to enable NL understanding)\n")
	}

	fmt.Print("Connect: ")
	if err := cmsClient.TestConnection(); err != nil {
		fmt.Printf("%sFAILED%s\n         %s\n", colorRed, colorReset, err)
		fmt.Println("\nCheck your CMS_TOKEN, CMS_WORKSPACE, and CMS_PROJECT values.")
		os.Exit(1)
	}
	fmt.Printf("%sOK%s\n", colorGreen, colorReset)

	fmt.Printf("\n%sCommands you can try:%s\n", colorBold, colorReset)
	fmt.Println("  show all items")
	fmt.Println("  list models")
	fmt.Println("  search for Tokyo")
	fmt.Println(`  create item title="New Place" description="A location"`)
	fmt.Println("  exit")
	fmt.Println()

	scanner := bufio.NewScanner(os.Stdin)
	for {
		fmt.Printf("%sYou:%s ", colorBold, colorReset)
		if !scanner.Scan() {
			break
		}

		input := strings.TrimSpace(scanner.Text())
		if input == "" {
			continue
		}
		if input == "exit" || input == "quit" || input == "q" {
			fmt.Println("Goodbye!")
			break
		}

		fmt.Println("Thinking...")
		response, err := chatHandler.Handle(input)
		if err != nil {
			fmt.Printf("\n%sError:%s %s\n\n", colorRed, colorReset, err)
			continue
		}
		fmt.Printf("\n%s\n", response)
	}
}

func mustEnv(key string) string {
	val := os.Getenv(key)
	if val == "" {
		fmt.Fprintf(os.Stderr, "%sError:%s missing required env var: %s\n", colorRed, colorReset, key)
		os.Exit(1)
	}
	return val
}
