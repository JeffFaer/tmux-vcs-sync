package tmux

import (
	"context"
	"fmt"
	"maps"
	"os"
	"slices"
	"strconv"
	"strings"

	"github.com/JeffFaer/go-stdlib-ext/morecmp"
	"github.com/JeffFaer/tmux-vcs-sync/api/exec"
)

var (
	tmux exec.Executable

	errNotTmux = fmt.Errorf("must be called from within a tmux session")
)

func init() {
	var err error
	tmux, err = exec.Lookup("tmux")
	if err != nil {
		panic(err)
	}
}

type Server interface {
	// PID returns the process ID of the server, if it's currently active.
	PID(context.Context) (int, error)

	// ListSessions lists the sessions that exist in this tmux server.
	ListSessions(context.Context) (Sessions, error)
	// ListClients lists all clients currently attached to this tmux server.
	ListClients(context.Context) ([]Client, error)

	// NewSession creates a new session in this tmux server.
	NewSession(context.Context, NewSessionOptions) (Session, error)
	// AttachOrSwitch either attaches the controlling terminal to the given TargetSession or switches the current tmux client to the TargetSession.
	AttachOrSwitch(context.Context, Session) error

	// Kill this tmux server.
	Kill(context.Context) error
}

// NewSessionOptions affects how NewSession creates sessions.
type NewSessionOptions struct {
	// Name is the optional initial name for the session.
	Name string
	// StartDir is the optional initial working directory for the session.
	StartDir string
}

func (opts NewSessionOptions) args() []string {
	var res []string
	if opts.Name != "" {
		res = append(res, []string{"-s", opts.Name}...)
	}
	if opts.StartDir != "" {
		res = append(res, []string{"-c", opts.StartDir}...)
	}
	return res
}

// Sessions is a list of Sessions, batched together so their operations are more
// performant.
type Sessions interface {
	// Server returns the tmux server that these Sessions belong to.
	Server() Server

	// Sessions returns each individual Session.
	Sessions() []Session

	// Property retrieves the value of the given property key for all of these
	// Sessions.
	Property(context.Context, SessionPropertyName) (map[Session]SessionPropertyValue, error)
	// Properties retrieves the values of all the given property keys for all of
	// these Sessions.
	Properties(context.Context, ...SessionPropertyName) (map[Session]SessionPropertyValues, error)
}

type Session interface {
	// Server returns the tmux server this Session belongs to.
	Server() Server
	// ID returns the ID of this session within its tmux server.
	ID() string

	// Property retrieves the value of the given property key.
	Property(context.Context, SessionPropertyName) (SessionPropertyValue, error)
	// Properties retrieves the values of all the given property keys.
	Properties(context.Context, ...SessionPropertyName) (SessionPropertyValues, error)

	// Rename this tmux session to have the given name.
	Rename(context.Context, string) error

	// Kill this tmux session.
	Kill(context.Context) error
}

// TODO: jfaer - This indirection is ripe for a refactor after go gets generic methods.
type SessionPropertyName interface {
	String() string
	iAmSessionPropertyName()
}
type SessionProperty[T any] string

const (
	SessionID       SessionProperty[string] = "#{session_id}"
	SessionName     SessionProperty[string] = "#{session_name}"
	SessionPath     SessionProperty[string] = "#{session_path}"
	SessionAttached SessionProperty[int]    = "#{session_attached}"
)

func (_ SessionProperty[T]) iAmSessionPropertyName() {}

func (prop SessionProperty[T]) Value(t T) SessionPropertyValue {
	var s string
	switch u := any(t).(type) {
	case string:
		s = u
	case int:
		s = strconv.Itoa(u)
	default:
		panic(fmt.Errorf("unsupported property type %T", u))
	}

	return SessionPropertyValue{
		name: SessionPropertyName(prop),
		val:  s,
	}
}

func (prop SessionProperty[T]) String() string {
	return string(prop)
}

type SessionPropertyValue struct {
	name SessionPropertyName
	val  string
}

type SessionPropertyValues map[SessionPropertyName]SessionPropertyValue

func CreateSessionPropertyValues(props ...SessionPropertyValue) SessionPropertyValues {
	ret := make(SessionPropertyValues, len(props))
	for _, prop := range props {
		ret.Set(prop)
	}
	return ret
}

func (vals SessionPropertyValues) Set(val SessionPropertyValue) {
	vals[val.name] = val
}

func PropertyValue[T any](property SessionProperty[T], val SessionPropertyValue) T {
	if val.name != SessionPropertyName(property) {
		panic(fmt.Errorf("PropertyValue called with the wrong SessionProperty: got %q want %q", property, val.name))
	}

	s := val.val
	var ret any
	switch any((*T)(nil)).(type) {
	case *string:
		ret = s
	case *int:
		i, err := strconv.Atoi(s)
		if err != nil {
			var zero T
			panic(fmt.Errorf("property %q expects type %T, got %q: %w", property, zero, s, err))
		}
		ret = i
	default:
		var zero T
		panic(fmt.Errorf("unsupported property type %T", zero))
	}
	return ret.(T)
}

func SinglePropertyValue[T any](property SessionProperty[T], vals SessionPropertyValues) T {
	val, ok := vals[SessionPropertyName(property)]
	if !ok {
		panic(fmt.Errorf("SinglePropertyValue called with an unknown SessionProperty: got %q want any of %q", property, slices.SortedFunc(maps.Keys(vals), morecmp.Comparing(SessionPropertyName.String))))
	}

	return PropertyValue(property, val)
}

type Client interface {
	// Property retrieves the value of the given property key.
	Property(context.Context, ClientProperty) (string, error)
	// Properties retrieves the values of all the given property keys.
	Properties(context.Context, ...ClientProperty) (map[ClientProperty]string, error)

	// DisplayMenu displays a menu in this client.
	DisplayMenu(context.Context, []MenuElement) error
}

type ClientProperty string

const (
	ClientTTY ClientProperty = "#{client_tty}"
)

type MenuElement interface {
	args() []string
}

// MenuEntry is an actual entry in the menu that has an executable command.
type MenuEntry struct {
	Name, Key, Command string
}

// MenuSpacer allows you to delineate sections within a menu.
type MenuSpacer struct{}

func (e MenuEntry) args() []string  { return []string{e.Name, e.Key, e.Command} }
func (e MenuSpacer) args() []string { return []string{""} }

type envVar struct {
	socketPath string
	pid        int
	sessionID  string
}

func getenv() (envVar, error) {
	env := os.Getenv("TMUX")
	if env == "" {
		return envVar{}, errNotTmux
	}

	sp := strings.SplitN(env, ",", 3)
	pid, err := strconv.Atoi(sp[1])
	if err != nil {
		return envVar{}, fmt.Errorf("%w: %w", errNotTmux, err)
	}
	return envVar{sp[0], pid, fmt.Sprintf("$%s", sp[2])}, nil
}

func (env envVar) server() *server {
	return &server{serverOptions{socketPath: env.socketPath}, tmux}
}

func (env envVar) session() *session {
	srv := env.server()
	return &session{srv, env.sessionID}
}
