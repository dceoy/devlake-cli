package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Client is a thin wrapper around the DevLake REST API.
type Client struct {
	BaseURL    string
	HTTPClient *http.Client
}

func NewClient(baseURL string) *Client {
	return &Client{
		BaseURL: strings.TrimRight(baseURL, "/"),
		HTTPClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// Generic request helpers

func (c *Client) get(path string) ([]byte, error) {
	resp, err := c.HTTPClient.Get(c.BaseURL + path)
	if err != nil {
		return nil, fmt.Errorf("GET %s: %w", path, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("GET %s: status %d: %s", path, resp.StatusCode, string(body))
	}
	return body, nil
}

func (c *Client) post(path string, payload interface{}) ([]byte, error) {
	var reqBody io.Reader
	if payload != nil {
		data, err := json.Marshal(payload)
		if err != nil {
			return nil, fmt.Errorf("marshaling payload: %w", err)
		}
		reqBody = strings.NewReader(string(data))
	}
	resp, err := c.HTTPClient.Post(c.BaseURL+path, "application/json", reqBody)
	if err != nil {
		return nil, fmt.Errorf("POST %s: %w", path, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("POST %s: status %d: %s", path, resp.StatusCode, string(body))
	}
	return body, nil
}

// Connection represents a DevLake data connection.
type Connection struct {
	ID   uint64 `json:"id"`
	Name string `json:"name"`
}

// Blueprint represents a DevLake blueprint.
type Blueprint struct {
	ID        uint64 `json:"id"`
	Name      string `json:"name"`
	Mode      string `json:"mode"`
	Enable    bool   `json:"enable"`
	CronConfig string `json:"cronConfig"`
}

// Pipeline represents a DevLake pipeline.
type Pipeline struct {
	ID        uint64 `json:"id"`
	Name      string `json:"name"`
	Status    string `json:"status"`
	Message   string `json:"message"`
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
}

// PipelineList is the response from listing pipelines.
type PipelineList struct {
	Count     int        `json:"count"`
	Pipelines []Pipeline `json:"pipelines"`
}

// ListConnections lists all connections for a given plugin.
func (c *Client) ListConnections(plugin string) ([]Connection, error) {
	body, err := c.get(fmt.Sprintf("/api/plugins/%s/connections", plugin))
	if err != nil {
		return nil, err
	}
	var conns []Connection
	if err := json.Unmarshal(body, &conns); err != nil {
		return nil, fmt.Errorf("parsing connections: %w", err)
	}
	return conns, nil
}

// ListBlueprints lists all blueprints.
func (c *Client) ListBlueprints() ([]Blueprint, error) {
	body, err := c.get("/api/blueprints")
	if err != nil {
		return nil, err
	}
	var resp struct {
		Blueprints []Blueprint `json:"blueprints"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("parsing blueprints: %w", err)
	}
	return resp.Blueprints, nil
}

// TriggerBlueprint triggers a blueprint by ID and returns the created pipeline.
func (c *Client) TriggerBlueprint(id uint64) (*Pipeline, error) {
	body, err := c.post(fmt.Sprintf("/api/blueprints/%d/trigger", id), nil)
	if err != nil {
		return nil, err
	}
	var p Pipeline
	if err := json.Unmarshal(body, &p); err != nil {
		return nil, fmt.Errorf("parsing pipeline: %w", err)
	}
	return &p, nil
}

// ListPipelines lists pipelines.
func (c *Client) ListPipelines() (*PipelineList, error) {
	body, err := c.get("/api/pipelines")
	if err != nil {
		return nil, err
	}
	var pl PipelineList
	if err := json.Unmarshal(body, &pl); err != nil {
		return nil, fmt.Errorf("parsing pipelines: %w", err)
	}
	return &pl, nil
}

// GetPipeline gets a pipeline by ID.
func (c *Client) GetPipeline(id uint64) (*Pipeline, error) {
	body, err := c.get(fmt.Sprintf("/api/pipelines/%d", id))
	if err != nil {
		return nil, err
	}
	var p Pipeline
	if err := json.Unmarshal(body, &p); err != nil {
		return nil, fmt.Errorf("parsing pipeline: %w", err)
	}
	return &p, nil
}
