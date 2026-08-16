//go:build !darwin && !linux

package domainverification

func canonicalDirectory(string) (string, error) { return "", ErrUnsafeConfig }

func readRegularFile(string, string, int64) ([]byte, error) { return nil, ErrNotFound }
