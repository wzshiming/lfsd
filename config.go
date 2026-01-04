package main

import "fmt"

// Config holds application configuration.
type Config struct {
	Listen      string
	Host        string
	Scheme      string
	ContentPath string
}

// ExternalURL returns the external URL for the server.
func (c *Config) ExternalURL() string {
	return fmt.Sprintf("%s://%s", c.Scheme, c.Host)
}
