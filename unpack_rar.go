package main

import (
	"context"
	"io"
	"os/exec"
	"path/filepath"
	"regexp"
	"syscall"
)

type cmdRAR struct {
	name    string
	command string
	ext     *regexp.Regexp
}

func (cmd *cmdRAR) Name() string {
	return cmd.name
}

// Unpack starts the unpacking process.
func (cmd *cmdRAR) Unpack(ctx context.Context, src, dest string, w io.Writer) error {

	tool := exec.CommandContext(ctx, cmd.command,
		"x", "-ai", "-c-", "-kb", "-o+", "-p-", "-y", "-v", src, dest)

	tool.Stdout = w
	tool.Stderr = w

	if err := tool.Start(); err != nil {
		return err
	}

	if err := tool.Wait(); err != nil {
		if exit, ok := err.(*exec.ExitError); ok {
			if exit.Sys().(syscall.WaitStatus).ExitStatus() != 10 {
				return ErrUnpackFailed
			}
		}
	}

	return nil
}

func (cmd *cmdRAR) CheckPath(path string) (string, bool) {
	directory := filepath.Dir(path) + string(filepath.Separator) + "."
	return directory, cmd.ext.MatchString(filepath.Ext(path))
}

func (cmd *cmdRAR) Installed() bool {
	_, err := exec.LookPath(cmd.command)
	return err == nil
}
