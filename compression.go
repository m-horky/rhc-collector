package insights

import (
	"bytes"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strings"
)

func Compress(directory string) (string, error) {
	archive := fmt.Sprintf("%s.tar.xz", directory)

	var stderr bytes.Buffer
	cmd := exec.Command("tar", "--create", "--xz", "--sparse", "--file", archive, directory)
	cmd.Stderr = &stderr

	slog.Debug("compressing", slog.String("command", strings.Join(cmd.Args, " ")))

	err := cmd.Run()
	if err != nil {
		slog.Error("compression failed", slog.Any("error", err), slog.Any("stderr", cmd.Stderr))
		return "", errors.New("compression failed")
	}

	stat, err := os.Stat(archive)
	if err != nil {
		slog.Error("could not inspect archive", slog.Any("error", err))
		return "", errors.New("could not analyze generated archive")
	}

	slog.Debug("archive created", slog.String("path", archive), slog.Int64("size (B)", stat.Size()))
	return archive, nil
}
