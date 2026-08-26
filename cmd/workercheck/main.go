// Command workercheck verifies that a packaged Growse executable can enter and
// cleanly leave both child-process modes without opening the GUI.
package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"time"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: workercheck <packaged-growse-executable>")
		os.Exit(2)
	}
	for _, mode := range []string{"GROWSE_RUNTIME_WORKER", "GROWSE_SERVICE_WORKER"} {
		if err := verifyMode(os.Args[1], mode); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}
}

func verifyMode(executable, mode string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, executable)
	command.Env = []string{mode + "=1", "GOMAXPROCS=1"}
	if runtime.GOOS == "windows" {
		for _, name := range []string{"SystemRoot", "WINDIR"} {
			if value := os.Getenv(name); value != "" {
				command.Env = append(command.Env, name+"="+value)
			}
		}
	}
	var stdout, stderr bytes.Buffer
	command.Stdin = bytes.NewReader(nil)
	command.Stdout = &stdout
	command.Stderr = &stderr
	err := command.Run()
	if ctx.Err() != nil {
		return fmt.Errorf("%s did not stop after stdin EOF: %w", mode, ctx.Err())
	}
	if err != nil {
		return fmt.Errorf("%s failed: %w: %s", mode, err, stderr.String())
	}
	if stdout.Len() != 0 || stderr.Len() != 0 {
		return fmt.Errorf("%s wrote unexpected output: stdout=%q stderr=%q", mode, stdout.String(), stderr.String())
	}
	return nil
}
