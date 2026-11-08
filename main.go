package main

import (
	"context"
	"os"
	"os/signal"
)

func main() { os.Exit(mainCode()) }
func mainCode() int {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, terminationSignal())
	defer stop()
	return run(ctx, os.Args[1:], os.Stdin, os.Stdout, os.Stderr)
}
