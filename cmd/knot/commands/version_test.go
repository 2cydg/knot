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

func TestVersionSubcommandsRegistered(t *testing.T) {
	if _, _, err := versionCmd.Find([]string{"check"}); err != nil {
		t.Fatalf("version check command not registered: %v", err)
	}
	if _, _, err := versionCmd.Find([]string{"upgrade"}); err != nil {
		t.Fatalf("version upgrade command not registered: %v", err)
	}
	if versionUpgradeCmd.Flag("yes") == nil {
		t.Fatal("version upgrade missing --yes flag")
	}
}

func TestRootUpgradeShortcutMatchesVersionUpgrade(t *testing.T) {
	if upgradeCmd.RunE == nil {
		t.Fatal("upgrade command missing RunE")
	}
	if versionUpgradeCmd.RunE == nil {
		t.Fatal("version upgrade command missing RunE")
	}
	if upgradeCmd.Flag("yes") == nil {
		t.Fatal("upgrade shortcut missing --yes flag")
	}
	if upgradeCmd.Use != versionUpgradeCmd.Use {
		t.Fatalf("upgrade Use = %q, want %q", upgradeCmd.Use, versionUpgradeCmd.Use)
	}
	if upgradeCmd.Short != versionUpgradeCmd.Short {
		t.Fatalf("upgrade Short = %q, want %q", upgradeCmd.Short, versionUpgradeCmd.Short)
	}
}
