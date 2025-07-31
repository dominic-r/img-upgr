package update

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/Masterminds/semver/v3"
	"gitlab.com/sdko-core/appli/img-upgr/pkg/docker"
	"gitlab.com/sdko-core/appli/img-upgr/pkg/logger"
)

const (
	// ImageTagPattern is the regex pattern for parsing image name and tag
	ImageTagPattern = `^([^:]+):(.+)$`
	// SemverTagPattern is the regex pattern for extracting prefix and semver from a tag
	SemverTagPattern = `^(.*?)(\d+\.\d+\.\d+)$`
)

// VersionInfo represents a tag with its parsed semantic version
type VersionInfo struct {
	FullTag string
	Version *semver.Version
}

// ImageInfo represents parsed information about a Docker image
type ImageInfo struct {
	Repository    string
	Tag           string
	Prefix        string
	Version       *semver.Version
	LatestTag     string
	LatestVersion *semver.Version
	HasUpdate     bool
}

// CheckImage checks if an image has an update available
func CheckImage(image string, dockerClient *docker.Client) (*ImageInfo, error) {
	logger.Debug("Checking image: %s", image)

	repo, tag, err := parseImageString(image)
	if err != nil {
		return nil, err
	}

	prefix, versionStr, err := extractVersionFromTag(tag)
	if err != nil {
		return nil, err
	}

	currentVer, err := semver.NewVersion(versionStr)
	if err != nil {
		logger.Debug("Invalid version: %s, error: %v", versionStr, err)
		return nil, fmt.Errorf("invalid semantic version: %s: %w", versionStr, err)
	}

	info := &ImageInfo{
		Repository: repo,
		Tag:        tag,
		Prefix:     prefix,
		Version:    currentVer,
	}

	latestVersion, err := findLatestVersion(repo, prefix, dockerClient)
	if err != nil {
		return nil, fmt.Errorf("failed to find latest version: %w", err)
	}

	if latestVersion != nil {
		info.LatestTag = latestVersion.FullTag
		info.LatestVersion = latestVersion.Version
		info.HasUpdate = latestVersion.Version.GreaterThan(currentVer)

		if info.HasUpdate {
			logger.Info("Update available for %s: %s → %s", repo, tag, latestVersion.FullTag)
		} else {
			logger.Debug("No update available for %s: %s is already the latest version", repo, tag)
		}
	}

	return info, nil
}

// parseImageString parses a Docker image string into repository and tag
func parseImageString(image string) (string, string, error) {
	re := regexp.MustCompile(ImageTagPattern)
	matches := re.FindStringSubmatch(image)
	if matches == nil {
		logger.Debug("No tag found in image: %s", image)
		return "", "", fmt.Errorf("no tag found in image: %s", image)
	}

	repo := matches[1]
	tag := matches[2]
	logger.Debug("Parsed repository: %s, tag: %s", repo, tag)
	return repo, tag, nil
}

// extractVersionFromTag extracts prefix and semver from a tag
func extractVersionFromTag(tag string) (string, string, error) {
	tagRe := regexp.MustCompile(SemverTagPattern)
	tagParts := tagRe.FindStringSubmatch(tag)
	if tagParts == nil {
		logger.Debug("Tag not semver-like: %s", tag)
		return "", "", fmt.Errorf("tag not semver-like: %s", tag)
	}

	prefix := tagParts[1]
	versionStr := tagParts[2]
	logger.Debug("Extracted prefix: '%s', version: %s", prefix, versionStr)
	return prefix, versionStr, nil
}

// findLatestVersion finds the latest version for a repository with a given prefix
func findLatestVersion(repo, prefix string, dockerClient *docker.Client) (*VersionInfo, error) {
	// Fetch all tags and find matching versions
	tags, err := dockerClient.FetchAllTags(repo)
	if err != nil {
		logger.Error("Failed to fetch tags: %v", err)
		return nil, fmt.Errorf("failed to fetch tags: %w", err)
	}

	matchedVersions := findMatchingVersions(tags, prefix)
	logger.Debug("Found %d matching versions", len(matchedVersions))

	if len(matchedVersions) == 0 {
		return nil, nil
	}

	// Sort by version descending
	sort.Slice(matchedVersions, func(i, j int) bool {
		return matchedVersions[i].Version.GreaterThan(matchedVersions[j].Version)
	})

	return &matchedVersions[0], nil
}

// findMatchingVersions finds all tags that match the prefix and can be parsed as semver
func findMatchingVersions(tags []string, prefix string) []VersionInfo {
	var matchedVersions []VersionInfo

	logger.Debug("Looking for tags with prefix: '%s'", prefix)
	for _, tag := range tags {
		if strings.HasPrefix(tag, prefix) {
			suffix := strings.TrimPrefix(tag, prefix)
			if version, err := semver.NewVersion(suffix); err == nil {
				logger.Debug("Found matching version: %s (parsed as %s)", tag, version)
				matchedVersions = append(matchedVersions, VersionInfo{
					FullTag: tag,
					Version: version,
				})
			}
		}
	}

	return matchedVersions
}
