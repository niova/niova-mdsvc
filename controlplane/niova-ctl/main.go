package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	log "github.com/sirupsen/logrus"

	ctlplfl "github.com/00pauln00/niova-mdsvc/controlplane/ctlplanefuncs/lib"
	"github.com/00pauln00/niova-mdsvc/controlplane/niova-ctl/ui"
)

func main() {
	cpEnabled := flag.Bool("cp", false, "Enable Control Plane client")
	cpRaftUUID := flag.String("raft-uuid", "", "Raft UUID for control plane client")
	cpGossipPath := flag.String("gossip-path", "", "Gossip node list path for control plane client")
	mdsvcURL := flag.String("mdsvc-url", "", "Base URL of an mdsvc-tidb server (e.g. http://localhost:8081) — when set, niova-ctl talks to this REST backend instead of the raft/gossip control plane, and -cp/-raft-uuid/-gossip-path are ignored")
	logFile := flag.String("log-file", ctlplfl.DefaultLogPath(), "Path to log file")
	flag.Parse()

	// Redirect logger output to log file to prevent screen corruption in TUI
	if *logFile != "" {
		f, err := os.OpenFile(*logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
		if err == nil {
			log.SetOutput(f)
			defer f.Close()
		} else {
			log.SetOutput(io.Discard)
		}
	} else {
		log.SetOutput(io.Discard)
	}

	app := ui.NewApp(*cpEnabled, *cpRaftUUID, *cpGossipPath, *logFile, *mdsvcURL)

	p := tea.NewProgram(app, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Printf("Error running niova-ctl TUI: %v\n", err)
		os.Exit(1)
	}
}
