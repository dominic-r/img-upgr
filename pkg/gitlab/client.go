package gitlab

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"gitlab.com/sdko-core/appli/img-upgr/pkg/config"
	"gitlab.com/sdko-core/appli/img-upgr/pkg/logger"
)

const (
	// DefaultTimeout is the default timeout for HTTP requests
	DefaultTimeout = 30 * time.Second
)

// Client represents a GitLab API client
type Client struct {
	baseURL    string
	token      string
	username   string
	repository string
	config     *config.Config
	httpClient *http.Client
}

// ClientOption defines a function that configures a Client
type ClientOption func(*Client)

// WithTimeout sets the HTTP client timeout
func WithTimeout(timeout time.Duration) ClientOption {
	return func(c *Client) {
		c.httpClient.Timeout = timeout
	}
}

// WithHTTPClient sets a custom HTTP client
func WithHTTPClient(httpClient *http.Client) ClientOption {
	return func(c *Client) {
		c.httpClient = httpClient
	}
}

// MergeRequestResponse represents the response from GitLab API when creating a merge request
type MergeRequestResponse struct {
	ID        int    `json:"id"`
	IID       int    `json:"iid"`
	WebURL    string `json:"web_url"`
	Title     string `json:"title"`
	State     string `json:"state"`
	CreatedAt string `json:"created_at"`
}

// NewClient creates a new GitLab client
func NewClient(cfg *config.Config, options ...ClientOption) (*Client, error) {
	logger.Debug("Creating new GitLab client")
	if err := cfg.ValidateGitLab(); err != nil {
		return nil, fmt.Errorf("GitLab configuration validation failed: %w", err)
	}

	// Extract base URL from repo URL
	parsedURL, err := url.Parse(cfg.GitLabRepo)
	if err != nil {
		return nil, fmt.Errorf("invalid repository URL: %w", err)
	}

	baseURL := fmt.Sprintf("%s://%s", parsedURL.Scheme, parsedURL.Host)
	logger.Debug("Using GitLab API base URL: %s", baseURL)

	client := &Client{
		baseURL:    baseURL,
		token:      cfg.GitLabToken,
		username:   cfg.GitLabUser,
		repository: cfg.GitLabRepo,
		config:     cfg,
		httpClient: &http.Client{
			Timeout: DefaultTimeout,
		},
	}

	// Apply options
	for _, option := range options {
		option(client)
	}

	return client, nil
}

// ProjectInfo contains information about a GitLab project
type ProjectInfo struct {
	Path     string
	Encoded  string
	FullPath string
}

// APIError represents an error returned by the GitLab API
type APIError struct {
	StatusCode int
	Message    string
	Response   map[string]interface{}
}

// Error returns the error message
func (e *APIError) Error() string {
	if e.Response != nil {
		return fmt.Sprintf("GitLab API error (status %d): %v", e.StatusCode, e.Response)
	}
	return fmt.Sprintf("GitLab API error (status %d): %s", e.StatusCode, e.Message)
}

// doRequest performs an HTTP request to the GitLab API and decodes the JSON response
func (c *Client) doRequest(ctx context.Context, method, path string, body interface{}, result interface{}) error {
	var reqBody io.Reader
	if body != nil {
		jsonData, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("error marshaling request body: %w", err)
		}
		reqBody = bytes.NewBuffer(jsonData)
	}

	// Create request with context
	req, err := http.NewRequestWithContext(ctx, method, path, reqBody)
	if err != nil {
		return fmt.Errorf("error creating request: %w", err)
	}

	// Set headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("PRIVATE-TOKEN", c.token)

	// Send request
	logger.Debug("Sending %s request to %s", method, path)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("error sending request: %w", err)
	}
	defer resp.Body.Close()

	// Check response status
	if resp.StatusCode >= 400 {
		var errorResp map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&errorResp); err != nil {
			return &APIError{
				StatusCode: resp.StatusCode,
				Message:    "failed to decode error response",
			}
		}
		return &APIError{
			StatusCode: resp.StatusCode,
			Response:   errorResp,
		}
	}

	// Parse response if result is provided
	if result != nil {
		if err := json.NewDecoder(resp.Body).Decode(result); err != nil {
			return fmt.Errorf("error parsing response: %w", err)
		}
	}

	return nil
}

// getProjectInfo extracts and formats project path information from repository URL
func (c *Client) getProjectInfo() (*ProjectInfo, error) {
	path := extractProjectPath(c.repository)
	if path == "" {
		return nil, fmt.Errorf("could not extract project path from repository URL")
	}

	return &ProjectInfo{
		Path:     path,
		Encoded:  url.PathEscape(path),
		FullPath: c.repository,
	}, nil
}

// CreateMergeRequest creates a new merge request in GitLab
func (c *Client) CreateMergeRequest(sourceBranch, targetBranch, title, description string) (*MergeRequestResponse, error) {
	return c.CreateMergeRequestWithContext(context.Background(), sourceBranch, targetBranch, title, description)
}

// CreateMergeRequestWithContext creates a new merge request in GitLab with context
func (c *Client) CreateMergeRequestWithContext(ctx context.Context, sourceBranch, targetBranch, title, description string) (*MergeRequestResponse, error) {
	logger.Info("Creating merge request from %s to %s: %s", sourceBranch, targetBranch, title)

	// Get project info
	projectInfo, err := c.getProjectInfo()
	if err != nil {
		return nil, err
	}
	logger.Debug("Using project path: %s", projectInfo.Path)

	// Build API URL
	apiURL := fmt.Sprintf("%s/api/v4/projects/%s/merge_requests",
		c.baseURL, projectInfo.Encoded)

	// Prepare request body
	requestBody := map[string]string{
		"source_branch": sourceBranch,
		"target_branch": targetBranch,
		"title":         title,
		"description":   description,
	}

	// Send request
	var mergeRequest MergeRequestResponse
	if err := c.doRequest(ctx, http.MethodPost, apiURL, requestBody, &mergeRequest); err != nil {
		logger.Error("Failed to create merge request: %v", err)
		return nil, fmt.Errorf("failed to create merge request: %w", err)
	}

	logger.Info("Merge request created successfully: %s", mergeRequest.WebURL)
	return &mergeRequest, nil
}

// extractProjectPath extracts the project path from a GitLab repository URL
func extractProjectPath(repoURL string) string {
	// Parse URL
	parsedURL, err := url.Parse(repoURL)
	if err != nil {
		logger.Error("Error parsing URL: %v", err)
		return ""
	}

	// Remove .git suffix if present
	path := parsedURL.Path
	path = strings.TrimSuffix(path, ".git")

	// Remove leading slash
	path = strings.TrimPrefix(path, "/")

	return path
}

// CreateBranch creates a new branch in GitLab
func (c *Client) CreateBranch(name, ref string) error {
	return c.CreateBranchWithContext(context.Background(), name, ref)
}

// CreateBranchWithContext creates a new branch in GitLab with context
func (c *Client) CreateBranchWithContext(ctx context.Context, name, ref string) error {
	logger.Info("Creating branch %s from %s", name, ref)

	// Get project info
	projectInfo, err := c.getProjectInfo()
	if err != nil {
		return err
	}

	// Build API URL
	apiURL := fmt.Sprintf("%s/api/v4/projects/%s/repository/branches",
		c.baseURL, projectInfo.Encoded)

	// Prepare request body
	requestBody := map[string]string{
		"branch": name,
		"ref":    ref,
	}

	// Send request
	if err := c.doRequest(ctx, http.MethodPost, apiURL, requestBody, nil); err != nil {
		logger.Error("Failed to create branch: %v", err)
		return fmt.Errorf("failed to create branch: %w", err)
	}

	logger.Info("Branch %s created successfully", name)
	return nil
}

// CommitFile commits a file change to GitLab
func (c *Client) CommitFile(branch, filePath, content, commitMessage string) error {
	return c.CommitFileWithContext(context.Background(), branch, filePath, content, commitMessage)
}

// CommitFileWithContext commits a file change to GitLab with context
func (c *Client) CommitFileWithContext(ctx context.Context, branch, filePath, content, commitMessage string) error {
	logger.Info("Committing file %s on branch %s", filePath, branch)

	// Get project info
	projectInfo, err := c.getProjectInfo()
	if err != nil {
		return err
	}

	// URL encode the file path
	encodedFilePath := url.PathEscape(filePath)

	// Build API URL
	apiURL := fmt.Sprintf("%s/api/v4/projects/%s/repository/files/%s",
		c.baseURL, projectInfo.Encoded, encodedFilePath)

	// Prepare request body
	requestBody := map[string]string{
		"branch":         branch,
		"content":        content,
		"commit_message": commitMessage,
	}

	// First try to update the file
	err = c.doRequest(ctx, http.MethodPut, apiURL, requestBody, nil)

	// If file doesn't exist (404), try to create it
	if apiErr, ok := err.(*APIError); ok && apiErr.StatusCode == http.StatusNotFound {
		logger.Debug("File not found, creating new file")
		err = c.doRequest(ctx, http.MethodPost, apiURL, requestBody, nil)
	}

	if err != nil {
		logger.Error("Failed to commit file: %v", err)
		return fmt.Errorf("failed to commit file: %w", err)
	}

	logger.Info("File %s committed successfully", filePath)
	return nil
}

// GetFile retrieves a file from GitLab
func (c *Client) GetFile(branch, filePath string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), c.httpClient.Timeout)
	defer cancel()

	// Get project info
	projectInfo, err := c.getProjectInfo()
	if err != nil {
		return "", err
	}

	// URL encode the file path
	encodedFilePath := url.PathEscape(filePath)

	// Build API URL
	apiURL := fmt.Sprintf("%s/api/v4/projects/%s/repository/files/%s/raw?ref=%s",
		c.baseURL, projectInfo.Encoded, encodedFilePath, url.QueryEscape(branch))

	// Create request
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return "", fmt.Errorf("error creating request: %w", err)
	}

	// Set headers
	req.Header.Set("PRIVATE-TOKEN", c.token)

	// Send request
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("error fetching file: %w", err)
	}
	defer resp.Body.Close()

	// Check response status
	if resp.StatusCode == http.StatusNotFound {
		return "", fmt.Errorf("file not found: %s", filePath)
	}

	if resp.StatusCode >= 400 {
		return "", &APIError{
			StatusCode: resp.StatusCode,
			Message:    fmt.Sprintf("failed to get file %s", filePath),
		}
	}

	// Read file content
	content, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("error reading file content: %w", err)
	}

	return string(content), nil
}
