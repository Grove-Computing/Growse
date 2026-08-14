package main

import (
	"fmt"
	"os"

	"github.com/Grove-Computing/Growse/internal/supplychain"
)

const growseModule = "github.com/Grove-Computing/Growse"

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: sbomcheck <spdx-json>")
		os.Exit(2)
	}

	file, err := os.Open(os.Args[1])
	if err != nil {
		fmt.Fprintf(os.Stderr, "open SBOM: %v\n", err)
		os.Exit(1)
	}
	defer file.Close()

	if err := supplychain.ValidateSPDX(file, growseModule); err != nil {
		fmt.Fprintf(os.Stderr, "invalid SBOM: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("SPDX SBOM検証成功: %s\n", os.Args[1])
}
