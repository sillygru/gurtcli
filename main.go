package main

import (
	"flag"
	"fmt"
	"os"

	tea "charm.land/bubbletea/v2"
	"github.com/sillygru/gurtcli/stats"
)

func main() {
	yolo := flag.Bool("yolo", false, "skip permission prompts")
	dangerous := flag.Bool("dangerously-skip-permissions", false, "skip permission prompts")
	modelFlag := flag.String("model", "", "model to use")
	providerFlag := flag.String("provider", "", "provider to use (openai, anthropic, custom)")
	reconfigure := flag.Bool("reconfigure", false, "force provider and model setup")
	forceLocal := flag.Bool("force-local", false, "use embedded llmdetails.json instead of fetching from GitHub")
	showVersion := flag.Bool("version", false, "print version and exit")
	debugFlag := flag.Bool("debug", false, "enable debug logging and resource monitor")
	flag.Parse()

	if *showVersion {
		fmt.Println(VersionString())
		os.Exit(0)
	}

	if flag.NArg() > 0 && flag.Arg(0) == "stats" {
		s, err := stats.Compute()
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		stats.Render(os.Stdout, s)
		os.Exit(0)
	}

	skipPerms := *yolo || *dangerous

	m := initialModel(skipPerms, *providerFlag, *modelFlag, *reconfigure, *forceLocal, *debugFlag)
	p := tea.NewProgram(m)
	globalProgram = p
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}
