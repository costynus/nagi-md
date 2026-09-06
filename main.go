package main

import (
	"fmt"
	"os"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: nmd <file>")
		os.Exit(1)
	}
	filename := os.Args[1]
	fmt.Printf("opening: %s\n", filename)
}
