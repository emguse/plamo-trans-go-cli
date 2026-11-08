package main

import (
	"os"
	"os/exec"
)

func configureProcess(cmd *exec.Cmd)             {}
func terminateProcess(cmd *exec.Cmd, force bool) { _ = cmd.Process.Kill() }

func terminationSignal() os.Signal { return os.Interrupt }
