//go:build windows

package updater

import (
	"archive/zip"
	"os"
	"path/filepath"
	"testing"
)

func TestExtractWindowsExecutable(t *testing.T) {
	archivePath := filepath.Join(t.TempDir(), "update.zip")
	archive, err := os.Create(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	writer := zip.NewWriter(archive)
	entry, err := writer.Create("viff-v0.1.1-windows-amd64/viff.exe")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := entry.Write([]byte("MZ-new-viff")); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}

	staged := filepath.Join(t.TempDir(), "viff-new.exe")
	if err := extractWindowsExecutable(archivePath, staged); err != nil {
		t.Fatalf("extractWindowsExecutable: %v", err)
	}
	got, err := os.ReadFile(staged)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "MZ-new-viff" {
		t.Fatalf("staged executable = %q", got)
	}
}

func TestExtractWindowsExecutableRejectsArchiveWithoutBinary(t *testing.T) {
	archivePath := filepath.Join(t.TempDir(), "update.zip")
	archive, err := os.Create(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	writer := zip.NewWriter(archive)
	if _, err := writer.Create("viff-v0.1.1-windows-amd64/README.md"); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}
	if err := extractWindowsExecutable(archivePath, filepath.Join(t.TempDir(), "viff-new.exe")); err == nil {
		t.Fatal("extractWindowsExecutable accepted an archive without viff.exe")
	}
}
