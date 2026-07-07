package main

import "fmt"

var Version = "1.0.3"
var CommitCount = "0"
var TelemetrySecret = ""

func VersionString() string {
	return fmt.Sprintf("gurtcli v%s+%s", Version, CommitCount)
}
