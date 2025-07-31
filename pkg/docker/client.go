package docker

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"gitlab.com/sdko-core/appli/img-upgr/pkg/logger"
)

const (
	// DefaultTimeout is the default timeout for HTTP requests
	DefaultTimeout = 30 * time.Second

	// DefaultPageSize is the default page size for Docker Hub API requests
	DefaultPageSize = 100

	// DockerHubAPIBaseURL is the base URL for Docker Hub API
	DockerHubAPIBaseURL = "https://hub.docker.com/v2/repositories"
)

// DockerHubTag represents a tag in Docker Hub
type DockerHubTag struct {
	Name        string    `json:"name"`
	LastUpdated time.Time `json:"last_updated,omitempty"`
	FullSize    int64     `json:"full_size,omitempty"`
}

// DockerHubResponse represents the response from Docker Hub API
type DockerHubResponse struct {
	Results []DockerHubTag `json:"results"`
	Next    string         `json:"next"`
	Count   int            `json:"count"`
}

// ClientOption is a function that configures a Client
type ClientOption func(*Client)

// WithTimeout sets the HTTP client timeout
func WithTimeout(timeout time.Duration) ClientOption {
	return func(c *Client) {
		c.httpClient.Timeout = timeout
	}
}

// WithPageSize sets the page size for API requests
func WithPageSize(pageSize int) ClientOption {
	return func(c *Client) {
		c.pageSize = pageSize
	}
}

// Client is a Docker Hub API client
type Client struct {
	httpClient *http.Client
	pageSize   int
	baseURL    string
}

// NewClient creates a new Docker Hub client with the given options
func NewClient(options ...ClientOption) *Client {
	client := &Client{
		httpClient: &http.Client{
			Timeout: DefaultTimeout,
		},
		pageSize: DefaultPageSize,
		baseURL:  DockerHubAPIBaseURL,
	}

	// Apply options
	for _, option := range options {
		option(client)
	}

	return client
}

// RepositoryInfo contains parsed information about a Docker repository
type RepositoryInfo struct {
	Namespace string
	Name      string
	FullName  string
}

// ParseRepositoryName parses a repository name into namespace and name
func ParseRepositoryName(repo string) RepositoryInfo {
	// Remove any tag information
	if idx := strings.Index(repo, ":"); idx > 0 {
		repo = repo[:idx]
	}

	split := strings.Split(repo, "/")
	if len(split) == 1 {
		return RepositoryInfo{
			Namespace: "library",
			Name:      split[0],
			FullName:  "library/" + split[0],
		}
	}

	return RepositoryInfo{
		Namespace: split[0],
		Name:      split[1],
		FullName:  repo,
	}
}

// FetchAllTags fetches all tags for a repository
func (c *Client) FetchAllTags(repo string) ([]string, error) {
	return c.FetchAllTagsWithContext(context.Background(), repo)
}

// FetchAllTagsWithContext fetches all tags for a repository with context
func (c *Client) FetchAllTagsWithContext(ctx context.Context, repo string) ([]string, error) {
	repoInfo := ParseRepositoryName(repo)
	url := fmt.Sprintf("%s/%s/%s/tags?page_size=%d", c.baseURL, repoInfo.Namespace, repoInfo.Name, c.pageSize)

	logger.Debug("Fetching tags for %s/%s", repoInfo.Namespace, repoInfo.Name)

	var tags []string
	pageCount := 0

	for url != "" {
		// Check if context is canceled
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		pageCount++
		logger.Debug("Fetching page %d from %s", pageCount, url)

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return nil, fmt.Errorf("error creating request: %w", err)
		}

		resp, err := c.httpClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("error fetching tags: %w", err)
		}

		// Check response status
		if resp.StatusCode != http.StatusOK {
			if err := resp.Body.Close(); err != nil {
				logger.Warn("Failed to close response body: %v", err)
			}
			return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
		}

		body, err := io.ReadAll(resp.Body)
		if err := resp.Body.Close(); err != nil {
			logger.Warn("Failed to close response body: %v", err)
		}
		if err != nil {
			return nil, fmt.Errorf("error reading response: %w", err)
		}

		var parsed DockerHubResponse
		if err := json.Unmarshal(body, &parsed); err != nil {
			return nil, fmt.Errorf("JSON parse error: %w", err)
		}

		for _, tag := range parsed.Results {
			tags = append(tags, tag.Name)
		}
		url = parsed.Next
		logger.Debug("Fetched %d tags so far", len(tags))
	}

	logger.Info("Found %d tags for %s", len(tags), repoInfo.FullName)
	return tags, nil
}

// FetchTagDetails fetches detailed information about a specific tag
func (c *Client) FetchTagDetails(repo, tag string) (*DockerHubTag, error) {
	ctx, cancel := context.WithTimeout(context.Background(), c.httpClient.Timeout)
	defer cancel()

	repoInfo := ParseRepositoryName(repo)
	url := fmt.Sprintf("%s/%s/%s/tags/%s", c.baseURL, repoInfo.Namespace, repoInfo.Name, tag)

	logger.Debug("Fetching details for tag %s in repository %s", tag, repoInfo.FullName)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("error creating request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error fetching tag details: %w", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			logger.Warn("Failed to close response body: %v", err)
		}
	}()

	// Check response status
	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("tag %s not found in repository %s", tag, repoInfo.FullName)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("error reading response: %w", err)
	}

	var tagDetails DockerHubTag
	if err := json.Unmarshal(body, &tagDetails); err != nil {
		return nil, fmt.Errorf("JSON parse error: %w", err)
	}

	return &tagDetails, nil
}
