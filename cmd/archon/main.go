package main

import (
	"fmt"
	"os"

	"github.com/availhealth/archon/internal/app"
)

var version = "dev"

func main() {
	if err := app.Run(os.Args[1:], version); err != nil {
		fmt.Fprintf(os.Stderr, "archon: %v\n", err)
		os.Exit(1)
	}
}
