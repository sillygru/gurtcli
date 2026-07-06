package main

import "fmt"

var Version = "0.15.1"
var CommitCount = "0"
var TelemetrySecret = ""

func VersionString() string {
	return fmt.Sprintf("gurtcli v%s+%s", Version, CommitCount)
}
