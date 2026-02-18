package docker

// Package docker provides integration with the Docker Engine via local socket.

import (
	"context"
	"fmt"

	"github.com/docker/docker/client"
)

// Client encapsulates the official Docker SDK client.
// We keep a single shared instance throughout the application.
type Client struct {
	cli *client.Client
}

// NewClient creates a connection to the Docker Engine using the default socket
// on the operating system:
//   - Linux/macOS: unix:///var/run/docker.sock
//   - Windows:     npipe:////./pipe/docker_engine
//
// Uses DOCKER_HOST if defined in the environment, allowing remote connection.
func NewClient() (*Client, error) {
	cli, err := client.NewClientWithOpts(
		client.FromEnv,
		client.WithAPIVersionNegotiation(), // negotiate version automatically with the daemon
	)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to Docker: %w", err)
	}

	return &Client{cli: cli}, nil
}

// Ping verifies if the Docker daemon is accessible.
// Useful to display connection status in the frontend.
func (c *Client) Ping(ctx context.Context) error {
	_, err := c.cli.Ping(ctx)
	if err != nil {
		return fmt.Errorf("Docker daemon inaccessible: %w", err)
	}
	return nil
}

// Close releases the client resources.
func (c *Client) Close() error {
	return c.cli.Close()
}

// Raw exposes the underlying client for use in other packages (containers, logs).
func (c *Client) Raw() *client.Client {
	return c.cli
}
