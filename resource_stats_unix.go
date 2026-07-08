//go:build !windows

package main

import (
	"runtime"
	"syscall"
	"time"

	tea "charm.land/bubbletea/v2"
)

var prevRusage syscall.Rusage
var prevWall time.Time

func resourceMonitorCmd() tea.Msg {
	var cur syscall.Rusage
	if err := syscall.Getrusage(syscall.RUSAGE_SELF, &cur); err != nil {
		return resourceStatsMsg{}
	}
	curWall := time.Now()

	var cpuPercent float64
	if !prevWall.IsZero() {
		curCPU := cur.Utime.Nano() + cur.Stime.Nano()
		prevCPU := prevRusage.Utime.Nano() + prevRusage.Stime.Nano()
		cpuDelta := curCPU - prevCPU
		wallDelta := curWall.Sub(prevWall).Nanoseconds()
		if wallDelta > 0 {
			cpuPercent = float64(cpuDelta) / float64(wallDelta) * 100
		}
	}
	prevRusage = cur
	prevWall = curWall

	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	memMB := float64(m.Sys) / 1024 / 1024

	return resourceStatsMsg{cpuPercent: cpuPercent, memMB: memMB}
}
