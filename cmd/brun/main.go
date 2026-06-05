package main

import (
	"os"

	"github.com/biotools/brun/internal/cli"
)

var version = "0.2.0"

func main() {
	if err := cli.Execute(cli.Options{Version: version}); err != nil {
		os.Exit(1)
	}
}
