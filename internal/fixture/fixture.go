// Package fixture locates the sample programs in the repository's test_input
// directory for tests. Tests must open fixtures only through this package.
package fixture

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// Sample directories under test_input.
const (
	FF      = "ff"      // Final Fantasy Extreme (VBGuard-protected)
	Bascode = "bascode" // BasCode for Windows 1.2
	Empire  = "empire"  // World Empire III
)

// root returns the absolute path of the test_input directory.
func root() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(file), "..", "..", "test_input")
}

// Dir returns the directory of a sample, failing the test if it is missing.
func Dir(t testing.TB, sample string) string {
	t.Helper()
	dir := filepath.Join(root(), sample)
	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		t.Fatalf("fixture directory test_input/%s is missing; see test_input/README.md", sample)
	}
	return dir
}

// Path returns the path of a file in a sample's directory, failing the test
// if it is missing.
func Path(t testing.TB, sample string, elem ...string) string {
	t.Helper()
	path := filepath.Join(append([]string{Dir(t, sample)}, elem...)...)
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("fixture test_input/%s is missing; see test_input/README.md", filepath.ToSlash(filepath.Join(append([]string{sample}, elem...)...)))
	}
	return path
}
