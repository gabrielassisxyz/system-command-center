package server

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"time"
)

const (
	defaultDockerVersion = "v1.41"
	defaultSocket        = "/var/run/docker.sock"
)

// DockerController abstracts container lifecycle operations (stop/restart)
// against the Docker Engine API. Tests inject a fake implementation.
type DockerController interface {
	Stop(ctx context.Context, containerID string) error
	Restart(ctx context.Context, containerID string) error
}

// NewDockerController creates a production DockerController that talks to the
// local Docker daemon over its Unix socket (or DOCKER_HOST). It issues plain
// HTTP requests to the Engine API, matching the minimal-client approach used
// by the collector package.
func NewDockerController() (DockerController, error) {
	host := os.Getenv("DOCKER_HOST")
	if host == "" {
		host = "unix://" + defaultSocket
	}
	addr, err := parseDockerHost(host)
	if err != nil {
		return nil, err
	}
	return &httpDockerController{
		client: &http.Client{
			Transport: &http.Transport{
				DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
					var d net.Dialer
					return d.DialContext(ctx, addr.network, addr.path)
				},
			},
			Timeout: 15 * time.Second,
		},
		version: defaultDockerVersion,
	}, nil
}

type dockerAddr struct {
	network string
	path    string
}

func parseDockerHost(host string) (dockerAddr, error) {
	switch {
	case len(host) >= 7 && host[:7] == "unix://":
		return dockerAddr{network: "unix", path: host[7:]}, nil
	case len(host) >= 6 && host[:6] == "tcp://":
		return dockerAddr{network: "tcp", path: host[6:]}, nil
	}
	return dockerAddr{}, fmt.Errorf("unsupported DOCKER_HOST: %s", host)
}

type httpDockerController struct {
	client  *http.Client
	version string
}

func (c *httpDockerController) Stop(ctx context.Context, containerID string) error {
	return c.postContainerAction(ctx, containerID, "stop")
}

func (c *httpDockerController) Restart(ctx context.Context, containerID string) error {
	return c.postContainerAction(ctx, containerID, "restart")
}

func (c *httpDockerController) postContainerAction(ctx context.Context, containerID, action string) error {
	if containerID == "" {
		return ErrContainerNotFound
	}
	u := fmt.Sprintf("http://docker/%s/containers/%s/%s", c.version, containerID, action)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, http.NoBody)
	if err != nil {
		return err
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == http.StatusNotFound {
		return ErrContainerNotFound
	}
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("docker %s %s: %s", action, containerID, resp.Status)
	}
	return nil
}
