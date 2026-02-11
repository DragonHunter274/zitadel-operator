package zitadel

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// Client wraps HTTP calls to the Zitadel Management/Admin API.
type Client struct {
	BaseURL    string
	Token      string
	HTTPClient *http.Client
}

// NewClient creates a new Zitadel API client.
func NewClient(baseURL, token string) *Client {
	return &Client{
		BaseURL: baseURL,
		Token:   token,
		HTTPClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

func (c *Client) do(ctx context.Context, method, path string, body interface{}, result interface{}, headers map[string]string) error {
	var bodyReader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshal request: %w", err)
		}
		bodyReader = bytes.NewReader(data)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.BaseURL+path, bodyReader)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode >= 400 {
		apiErr := &APIError{StatusCode: resp.StatusCode}
		_ = json.Unmarshal(respBody, apiErr)
		if apiErr.Message == "" {
			apiErr.Message = string(respBody)
		}
		return apiErr
	}

	if result != nil && len(respBody) > 0 {
		if err := json.Unmarshal(respBody, result); err != nil {
			return fmt.Errorf("unmarshal response: %w", err)
		}
	}

	return nil
}

func (c *Client) orgHeaders(orgID string) map[string]string {
	if orgID == "" {
		return nil
	}
	return map[string]string{"x-zitadel-orgid": orgID}
}

// --- Organizations ---

func (c *Client) CreateOrganization(ctx context.Context, name string) (*OrgResponse, error) {
	var resp OrgResponse
	err := c.do(ctx, http.MethodPost, "/admin/v1/orgs", &CreateOrgRequest{Name: name}, &resp, nil)
	return &resp, err
}

func (c *Client) DeleteOrganization(ctx context.Context, orgID string) error {
	return c.do(ctx, http.MethodDelete, "/admin/v1/orgs/"+orgID, nil, nil, nil)
}

// --- Projects ---

func (c *Client) CreateProject(ctx context.Context, orgID string, req CreateProjectRequest) (*ProjectResponse, error) {
	var resp ProjectResponse
	err := c.do(ctx, http.MethodPost, "/management/v1/projects", &req, &resp, c.orgHeaders(orgID))
	return &resp, err
}

func (c *Client) UpdateProject(ctx context.Context, orgID, projectID string, req UpdateProjectRequest) error {
	return c.do(ctx, http.MethodPut, "/management/v1/projects/"+projectID, &req, nil, c.orgHeaders(orgID))
}

func (c *Client) DeleteProject(ctx context.Context, orgID, projectID string) error {
	return c.do(ctx, http.MethodDelete, "/management/v1/projects/"+projectID, nil, nil, c.orgHeaders(orgID))
}

// --- Applications ---

func (c *Client) CreateOIDCApp(ctx context.Context, orgID, projectID string, req CreateOIDCAppRequest) (*AppResponse, error) {
	var resp AppResponse
	err := c.do(ctx, http.MethodPost, "/management/v1/projects/"+projectID+"/apps/oidc", &req, &resp, c.orgHeaders(orgID))
	return &resp, err
}

func (c *Client) CreateAPIApp(ctx context.Context, orgID, projectID string, req CreateAPIAppRequest) (*AppResponse, error) {
	var resp AppResponse
	err := c.do(ctx, http.MethodPost, "/management/v1/projects/"+projectID+"/apps/api", &req, &resp, c.orgHeaders(orgID))
	return &resp, err
}

func (c *Client) CreateSAMLApp(ctx context.Context, orgID, projectID string, req CreateSAMLAppRequest) (*AppResponse, error) {
	var resp AppResponse
	err := c.do(ctx, http.MethodPost, "/management/v1/projects/"+projectID+"/apps/saml", &req, &resp, c.orgHeaders(orgID))
	return &resp, err
}

func (c *Client) DeleteApp(ctx context.Context, orgID, projectID, appID string) error {
	return c.do(ctx, http.MethodDelete, "/management/v1/projects/"+projectID+"/apps/"+appID, nil, nil, c.orgHeaders(orgID))
}

// --- Users ---

func (c *Client) CreateHumanUser(ctx context.Context, orgID string, req CreateHumanUserRequest) (*UserResponse, error) {
	var resp UserResponse
	err := c.do(ctx, http.MethodPost, "/management/v1/users/human", &req, &resp, c.orgHeaders(orgID))
	return &resp, err
}

func (c *Client) CreateMachineUser(ctx context.Context, orgID string, req CreateMachineUserRequest) (*UserResponse, error) {
	var resp UserResponse
	err := c.do(ctx, http.MethodPost, "/management/v1/users/machine", &req, &resp, c.orgHeaders(orgID))
	return &resp, err
}

func (c *Client) DeleteUser(ctx context.Context, orgID, userID string) error {
	return c.do(ctx, http.MethodDelete, "/management/v1/users/"+userID, nil, nil, c.orgHeaders(orgID))
}

func (c *Client) CreatePAT(ctx context.Context, orgID, userID string) (*PATResponse, error) {
	var resp PATResponse
	err := c.do(ctx, http.MethodPost, "/management/v1/users/"+userID+"/pats", &struct{}{}, &resp, c.orgHeaders(orgID))
	return &resp, err
}

// --- Roles ---

func (c *Client) BulkAddProjectRoles(ctx context.Context, orgID, projectID string, roles []RoleEntry) error {
	return c.do(ctx, http.MethodPost, "/management/v1/projects/"+projectID+"/roles/_bulk", &BulkAddRolesRequest{Roles: roles}, nil, c.orgHeaders(orgID))
}

func (c *Client) ListProjectRoles(ctx context.Context, orgID, projectID string) ([]RoleResult, error) {
	var resp ListRolesResponse
	err := c.do(ctx, http.MethodPost, "/management/v1/projects/"+projectID+"/roles/_search", &struct{}{}, &resp, c.orgHeaders(orgID))
	return resp.Result, err
}

func (c *Client) RemoveProjectRole(ctx context.Context, orgID, projectID, roleKey string) error {
	return c.do(ctx, http.MethodDelete, "/management/v1/projects/"+projectID+"/roles/"+roleKey, nil, nil, c.orgHeaders(orgID))
}

// --- Auth ---

// GetPasswordToken authenticates with Zitadel using the Resource Owner Password Grant
// and returns a bearer token. Used for bootstrapping the operator service account.
func GetPasswordToken(ctx context.Context, baseURL, username, password string) (string, error) {
	data := url.Values{
		"grant_type": {"urn:ietf:params:oauth:grant-type:resource_owner_password_credentials"},
		"username":   {username},
		"password":   {password},
		"scope":      {"openid urn:zitadel:iam:org:project:id:zitadel:aud"},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/oauth/v2/token",
		bytes.NewBufferString(data.Encode()))
	if err != nil {
		return "", fmt.Errorf("create token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := (&http.Client{Timeout: 30 * time.Second}).Do(req)
	if err != nil {
		return "", fmt.Errorf("token request: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("token request failed (HTTP %d): %s", resp.StatusCode, string(body))
	}

	var tokenResp struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return "", fmt.Errorf("parse token response: %w", err)
	}
	if tokenResp.AccessToken == "" {
		return "", fmt.Errorf("empty access token in response")
	}
	return tokenResp.AccessToken, nil
}

// --- Health ---

func (c *Client) IsReady(ctx context.Context) bool {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+"/debug/ready", nil)
	if err != nil {
		return false
	}
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}
