package main

import (
	"context"
	"fmt"
	"os"

	"github.com/augusttw/procscope/internal/cli"
)

func main() {
	if err := cli.New().Run(context.Background(), os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "procscope:", err)
		os.Exit(1)
	}
}
