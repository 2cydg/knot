package update

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

var ErrDevBuild = errors.New("development build does not support self-upgrade")

type CheckResult struct {
	CurrentVersion  string `json:"current_version"`
	LatestVersion   string `json:"latest_version,omitempty"`
	UpdateAvailable bool   `json:"update_available"`
	Channel         string `json:"channel,omitempty"`
	NotesURL        string `json:"notes_url,omitempty"`
	AssetKey        string `json:"asset,omitempty"`
	Upgradable      bool   `json:"upgradable"`
	Reason          string `json:"reason,omitempty"`
	Manifest        *Manifest
}

func CheckLatest(ctx context.Context, client *Client, currentVersion, goos, goarch string) (*CheckResult, error) {
	if client == nil {
		client = NewClient()
	}
	manifest, err := client.FetchLatest(ctx)
	if err != nil {
		return nil, err
	}
	assetKey, _, err := manifest.AssetFor(goos, goarch)
	if err != nil {
		return nil, err
	}
	upgradable, err := IsUpgradable(currentVersion, manifest.Version)
	if err != nil {
		return nil, err
	}
	return &CheckResult{
		CurrentVersion:  currentVersion,
		LatestVersion:   manifest.Version,
		UpdateAvailable: upgradable,
		Channel:         manifest.Channel,
		NotesURL:        manifest.NotesURL,
		AssetKey:        assetKey,
		Upgradable:      true,
		Manifest:        manifest,
	}, nil
}

func IsUpgradable(current, latest string) (bool, error) {
	if current == "dev" {
		return true, nil
	}
	c, err := parseSemver(current)
	if err != nil {
		return false, fmt.Errorf("invalid current version %q: %w", current, err)
	}
	l, err := parseSemver(latest)
	if err != nil {
		return false, fmt.Errorf("invalid latest version %q: %w", latest, err)
	}
	return l.compare(c) > 0, nil
}

var semverPattern = regexp.MustCompile(`^v([0-9]+)\.([0-9]+)\.([0-9]+)(?:-([0-9A-Za-z.-]+))?$`)

type semver struct {
	major int
	minor int
	patch int
	pre   string
}

func parseSemver(value string) (semver, error) {
	matches := semverPattern.FindStringSubmatch(value)
	if matches == nil {
		return semver{}, errors.New("expected vMAJOR.MINOR.PATCH")
	}
	major, err := strconv.Atoi(matches[1])
	if err != nil {
		return semver{}, err
	}
	minor, err := strconv.Atoi(matches[2])
	if err != nil {
		return semver{}, err
	}
	patch, err := strconv.Atoi(matches[3])
	if err != nil {
		return semver{}, err
	}
	pre := matches[4]
	if pre != "" && strings.Contains(pre, "..") {
		return semver{}, errors.New("invalid prerelease suffix")
	}
	return semver{major: major, minor: minor, patch: patch, pre: pre}, nil
}

func (v semver) compare(other semver) int {
	if v.major != other.major {
		return compareInt(v.major, other.major)
	}
	if v.minor != other.minor {
		return compareInt(v.minor, other.minor)
	}
	if v.patch != other.patch {
		return compareInt(v.patch, other.patch)
	}
	return comparePrerelease(v.pre, other.pre)
}

func compareInt(a, b int) int {
	switch {
	case a > b:
		return 1
	case a < b:
		return -1
	default:
		return 0
	}
}

func comparePrerelease(a, b string) int {
	if a == b {
		return 0
	}
	if a == "" {
		return 1
	}
	if b == "" {
		return -1
	}
	aParts := strings.Split(a, ".")
	bParts := strings.Split(b, ".")
	for i := 0; i < len(aParts) && i < len(bParts); i++ {
		if aParts[i] == bParts[i] {
			continue
		}
		aNum, aErr := strconv.Atoi(aParts[i])
		bNum, bErr := strconv.Atoi(bParts[i])
		switch {
		case aErr == nil && bErr == nil:
			return compareInt(aNum, bNum)
		case aErr == nil:
			return -1
		case bErr == nil:
			return 1
		case aParts[i] > bParts[i]:
			return 1
		default:
			return -1
		}
	}
	return compareInt(len(aParts), len(bParts))
}
