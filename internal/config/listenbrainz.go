package config

import (
	"net/url"
	"strings"

	"go.yaml.in/yaml/v3"
)

// defaultListenBrainz is the ListenBrainz API asked unless url names another.
const defaultListenBrainz = "https://api.listenbrainz.org"

// ListenBrainz is the listenbrainz: block. A user turns Recommendations
// on; URL names a self-hosted ListenBrainz. A bad value is a Problem and
// leaves Recommendations off.
type ListenBrainz struct {
	User     string
	URL      string
	Problems []Problem
	bad      bool
}

// On reports whether navigo fetches Recommendations.
func (l ListenBrainz) On() bool { return l.User != "" && !l.bad }

// API is the base URL of the ListenBrainz API, without a trailing slash.
func (l ListenBrainz) API() string {
	if l.URL == "" {
		return defaultListenBrainz
	}
	return strings.TrimRight(l.URL, "/")
}

// UnmarshalYAML reads the block key by key, so a bad value is a Problem at
// its place and not an error.
func (l *ListenBrainz) UnmarshalYAML(n *yaml.Node) error {
	bad := func(at *yaml.Node, msg string) {
		l.Problems = append(l.Problems, Problem{pos(at), msg})
		l.bad = true
	}
	if n.Kind != yaml.MappingNode {
		bad(n, "listenbrainz: give a block with user and, if you like, url")
		return nil
	}
	for i := 0; i+1 < len(n.Content); i += 2 {
		key, val := n.Content[i], n.Content[i+1]
		switch key.Value {
		case "user":
			if val.Kind != yaml.ScalarNode || strings.TrimSpace(val.Value) == "" {
				bad(val, "listenbrainz.user: give your ListenBrainz user name")
				continue
			}
			l.User = strings.TrimSpace(val.Value)
		case "url":
			u, err := url.Parse(val.Value)
			if val.Kind != yaml.ScalarNode || err != nil || u.Host == "" ||
				u.Scheme != "http" && u.Scheme != "https" {
				bad(
					val,
					"listenbrainz.url: give an http or https URL, such as "+defaultListenBrainz,
				)
				continue
			}
			l.URL = val.Value
		}
	}
	return nil
}
