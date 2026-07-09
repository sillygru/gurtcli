package main

import "fmt"

var Version = "1.1.5"
var CommitCount = "0"
var TelemetrySecret = ""

func VersionString() string {
	return fmt.Sprintf("gurtcli v%s", Version)
}
