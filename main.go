package main

import (
	"github.com/glassflow/glassflow-cli/cmd"
)

var (
	version = "dev"
	commit  = "unknown"
	date    = "unknown"
)

func main() {
	cmd.Execute(version, commit, date)
}
