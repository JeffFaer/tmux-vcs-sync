package tmux

import (
	"context"
	"fmt"
	"strings"
)

// A placeholder value to indicate that commands should be run in the "current"
// client.
// That means that we don't need to specify a client at all in the commands
// since tmux will do the leg work of figuring out which client we are.
const currentClientTTY = "<<current>>"

type client struct {
	srv *server
	tty string
}

// CurrentClient returns a Client if this terminal is currently a tmux client.
func CurrentClient() (Client, error) {
	c := MaybeCurrentClient()
	if c == nil {
		return nil, errNotTmux
	}
	return c, nil
}

// MaybeCurrentClient returns a Client if the terminal is currently a tmux
// client. If it's not currently a tmux client, returns nil.
func MaybeCurrentClient() Client {
	env, err := getenv()
	if err != nil {
		return nil
	}
	return &client{env.server(), currentClientTTY}
}

func (c *client) Property(ctx context.Context, prop ClientProperty) (string, error) {
	props, err := c.Properties(ctx, prop)
	if err != nil {
		return "", err
	}
	return props[prop], nil
}

func (c *client) Properties(ctx context.Context, props ...ClientProperty) (map[ClientProperty]string, error) {
	res, err := properties(props, func(keys []string) ([]string, error) {
		args := []string{"display-message", "-p", "-F", strings.Join(keys, "\n")}
		if c.tty != currentClientTTY {
			args = append(args, "-t", c.tty)
		}
		stdout, err := c.srv.command(ctx, args...).RunStdout()
		if err != nil {
			return nil, err
		}
		return strings.Split(stdout, "\n"), nil
	})
	if err != nil {
		return nil, fmt.Errorf("client %s: %w", c.tty, err)
	}
	return res, nil
}

func (c *client) DisplayMenu(ctx context.Context, elems []MenuElement) error {
	args := []string{"display-menu"}
	if c.tty != currentClientTTY {
		args = append(args, "-c", c.tty)
	}
	for _, e := range elems {
		args = append(args, e.args()...)
	}
	return c.srv.command(ctx, args...).Run()
}
