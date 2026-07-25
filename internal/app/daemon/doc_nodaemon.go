//go:build nodaemon

// Package daemon is empty in `nodaemon` builds: serve/daemon mode is compiled
// out and nothing imports this package. The file exists only so the directory
// still declares a buildable package under the tag.
package daemon
