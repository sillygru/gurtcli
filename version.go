package main

import "fmt"

var Version = "0.14.1"
var CommitCount = "0"

func VersionString() string {
	return fmt.Sprintf("gurtcli v%s+%s", Version, CommitCount)
}
