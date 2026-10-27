package main

import (
	"fmt"
	"os"
)

func main() {
	if len(os.Args) > 1 {
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", os.Args[1])
		os.Exit(2)
	}

	fmt.Println("GoRelix")
	fmt.Println("Relational database engine")
	fmt.Println("Status: initial development")
}