package commands

import "testing"

func TestCurrentVersionInfoIncludesRuntimeTarget(t *testing.T) {
	info := currentVersionInfo()

	if info.Version == "" {
		t.Fatal("version must not be empty")
	}
	if info.OS == "" {
		t.Fatal("os must not be empty")
	}
	if info.Arch == "" {
		t.Fatal("arch must not be empty")
	}
}
