package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

const BaseURL = "https://api.cms.reearth.io"

type Client struct {
	token     string
	workspace string
	project   string
	model     string
	http      *http.Client
}

func New(token, workspace, project, model string) *Client {
	return &Client{
		token:     token,
		workspace: workspace,
		project:   project,
		model:     model,
		http:      &http.Client{Timeout: 15 * time.Second},
	}
}

// --- Types ---

type ItemsResponse struct {
	Items      []Item `json:"items"`
	TotalCount int    `json:"totalCount"`
	Page       int    `json:"page"`
	PerPage    int    `json:"perPage"`
}

type Item struct {
	ID        string  `json:"id"`
	Fields    []Field `json:"fields"`
	Status    string  `json:"status"`
	CreatedAt string  `json:"createdAt"`
	UpdatedAt string  `json:"updatedAt"`
}

type Field struct {
	ID    string      `json:"id"`
	Key   string      `json:"key"`
	Type  string      `json:"type"`
	Value interface{} `json:"value"`
}

type SchemaField struct {
	ID    string `json:"id"`
	Type  string `json:"type"`
	Key   string `json:"key"`
	Title string `json:"title"`
}

type SchemaFields struct {
	Fields []SchemaField `json:"fields"`
}

type Model struct {
	ID          string       `json:"id"`
	Name        string       `json:"name"`
	Key         string       `json:"key"`
	Description string       `json:"description"`
	Schema      SchemaFields `json:"schema"`
}

type ModelsResponse struct {
	Models []Model `json:"models"`
}


// --- HTTP helper ---

func (c *Client) do(method, path string, body []byte) ([]byte, error) {
	var bodyReader io.Reader
	if body != nil {
		bodyReader = bytes.NewReader(body)
	}

	req, err := http.NewRequest(method, BaseURL+path, bodyReader)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("API error %d: %s", resp.StatusCode, string(respBody))
	}
	return respBody, nil
}

// --- API methods ---

func (c *Client) GetItems(page, perPage int) (*ItemsResponse, error) {
	path := fmt.Sprintf("/%s/projects/%s/models/%s/items?page=%d&perPage=%d",
		c.workspace, c.project, c.model, page, perPage)
	body, err := c.do("GET", path, nil)
	if err != nil {
		return nil, err
	}
	var result ItemsResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("parse error: %w", err)
	}
	return &result, nil
}

func (c *Client) SearchItems(keyword string, page, perPage int) (*ItemsResponse, error) {
	path := fmt.Sprintf("/%s/projects/%s/models/%s/items?page=%d&perPage=%d&keyword=%s",
		c.workspace, c.project, c.model, page, perPage, url.QueryEscape(keyword))
	body, err := c.do("GET", path, nil)
	if err != nil {
		return nil, err
	}
	var result ItemsResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("parse error: %w", err)
	}
	return &result, nil
}

func (c *Client) GetModels() (*ModelsResponse, error) {
	path := fmt.Sprintf("/%s/projects/%s/models", c.workspace, c.project)
	body, err := c.do("GET", path, nil)
	if err != nil {
		return nil, err
	}
	var result ModelsResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("parse error: %w", err)
	}
	return &result, nil
}

// TestConnection verifies the token and project are reachable.
func (c *Client) TestConnection() error {
	_, err := c.GetModels()
	return err
}

func (c *Client) Info() string {
	return fmt.Sprintf("Workspace: %s | Project: %s | Model: %s", c.workspace, c.project, c.model)
}
