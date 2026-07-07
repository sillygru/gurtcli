//go:build windows

package main

import (
	"runtime"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

var prevWall time.Time

func resourceMonitorCmd() tea.Msg {
	curWall := time.Now()
	prevWall = curWall

	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	memMB := float64(m.Sys) / 1024 / 1024

	return resourceStatsMsg{cpuPercent: 0, memMB: memMB}
}
