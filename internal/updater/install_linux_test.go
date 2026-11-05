//go:build linux

package updater

import (
	"archive/tar"
	"compress/gzip"
	"os"
	"path/filepath"
	"testing"
)

func TestExtractLinuxExecutable(t *testing.T) {
	archivePath := filepath.Join(t.TempDir(), "update.tar.gz")
	file, err := os.Create(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	gz := gzip.NewWriter(file)
	writer := tar.NewWriter(gz)
	payload := []byte("#!/bin/sh\necho viff\n")
	if err := writer.WriteHeader(&tar.Header{Name: "viff-v0.1.1-linux-amd64/viff", Mode: 0o755, Size: int64(len(payload))}); err != nil {
		t.Fatal(err)
	}
	if _, err := writer.Write(payload); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}

	staged := filepath.Join(t.TempDir(), "viff-new")
	if err := extractLinuxExecutable(archivePath, staged); err != nil {
		t.Fatalf("extractLinuxExecutable: %v", err)
	}
	got, err := os.ReadFile(staged)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(payload) {
		t.Fatalf("staged binary = %q", got)
	}
}

func TestExtractLinuxExecutableRejectsArchiveWithoutBinary(t *testing.T) {
	archivePath := filepath.Join(t.TempDir(), "update.tar.gz")
	file, err := os.Create(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	gz := gzip.NewWriter(file)
	writer := tar.NewWriter(gz)
	if err := writer.WriteHeader(&tar.Header{Name: "viff-v0.1.1-linux-amd64/README.md", Mode: 0o644, Size: 2}); err != nil {
		t.Fatal(err)
	}
	if _, err := writer.Write([]byte("hi")); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	if err := extractLinuxExecutable(archivePath, filepath.Join(t.TempDir(), "viff-new")); err == nil {
		t.Fatal("extractLinuxExecutable accepted an archive without viff")
	}
}
