package main

import (
	"os"

	"github.com/robinjoseph08/wktr/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}
