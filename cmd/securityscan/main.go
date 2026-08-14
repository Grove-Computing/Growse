package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/Grove-Computing/Growse/internal/securityscan"
)

func main() {
	root := flag.String("root", ".", "repository root")
	config := flag.String("config", ".security/security-policy.json", "security policy path")
	diffBase := flag.String("diff", "", "scan files changed since this base revision")
	flag.Parse()

	policyPath := *config
	if !filepath.IsAbs(policyPath) {
		policyPath = filepath.Join(*root, policyPath)
	}
	policy, err := securityscan.LoadPolicy(policyPath, time.Now())
	if err != nil {
		fail(err)
	}
	paths, err := securityscan.GitFiles(*root, *diffBase)
	if err != nil {
		fail(err)
	}
	findings, err := (securityscan.Scanner{Root: *root, Policy: policy}).Scan(paths)
	if err != nil {
		fail(err)
	}
	for _, finding := range findings {
		fmt.Fprintln(os.Stderr, securityscan.FormatFinding(finding, os.Getenv("GITHUB_ACTIONS") == "true"))
	}
	if len(findings) > 0 {
		fmt.Fprintf(os.Stderr, "security scan failed with %d finding(s)\n", len(findings))
		os.Exit(1)
	}
	fmt.Printf("security scan passed for %d tracked file(s)\n", len(paths))
}

func fail(err error) {
	fmt.Fprintf(os.Stderr, "security scan failed: %v\n", err)
	os.Exit(1)
}
