//go:build darwin || linux

package domainverification

import (
	"io"
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"
)

func canonicalDirectory(directory string) (string, error) {
	info, err := os.Lstat(directory)
	if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return "", ErrUnsafeConfig
	}
	root, err := filepath.EvalSymlinks(directory)
	if err != nil || !filepath.IsAbs(root) || filepath.Clean(root) != root {
		return "", ErrUnsafeConfig
	}
	return root, nil
}

func readRegularFile(root, filename string, maximum int64) ([]byte, error) {
	directoryFD, err := unix.Open(root, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, ErrNotFound
	}
	defer unix.Close(directoryFD)
	fileFD, err := unix.Openat(directoryFD, filename, unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, ErrNotFound
	}
	file := os.NewFile(uintptr(fileFD), filename)
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() > maximum {
		return nil, ErrNotFound
	}
	content, err := io.ReadAll(io.LimitReader(file, maximum+1))
	if err != nil || int64(len(content)) > maximum || !utf8Valid(content) {
		return nil, ErrNotFound
	}
	return content, nil
}
