package unpacker

import (
	"context"
	"io"
	"os/exec"
	"path/filepath"
	"regexp"
)

type cmdZIP struct {
	name    string
	command string
	ext     *regexp.Regexp
}

func (cmd *cmdZIP) Name() string {
	return cmd.name
}

func (cmd *cmdZIP) Unpack(ctx context.Context, src, dest string, w io.Writer) error {

	tool := exec.CommandContext(ctx, cmd.command, "-o", src, "-d", dest)
	tool.Stdout = w
	tool.Stderr = w

	if err := tool.Start(); err != nil {
		return err
	}

	return tool.Wait()
}

func (cmd *cmdZIP) CheckPath(path string) (string, bool) {
	return path, cmd.ext.MatchString(filepath.Ext(path))
}

func (cmd *cmdZIP) Installed() bool {
	_, err := exec.LookPath(cmd.command)
	return err == nil
}
