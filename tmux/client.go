package tmux

import (
	"fmt"
	"strings"
)

type Client struct {
	srv *Server
	TTY string
}

func (c *Client) Property(prop ClientProperty) (string, error) {
	props, err := c.Properties(prop)
	if err != nil {
		return "", err
	}
	return props[prop], nil
}

func (c *Client) Properties(props ...ClientProperty) (map[ClientProperty]string, error) {
	res, err := properties(props, func(keys []string) ([]string, error) {
		stdout, err := c.srv.command("display-message", "-t", c.TTY, "-p", "-F", strings.Join(keys, "\n")).RunStdout()
		if err != nil {
			return nil, err
		}
		return strings.Split(stdout, "\n"), nil
	})
	if err != nil {
		return nil, fmt.Errorf("client %s: %w", c.TTY, err)
	}
	return res, nil
}

type ClientProperty string

const (
	ClientTTY ClientProperty = "#{client_tty}"
)
