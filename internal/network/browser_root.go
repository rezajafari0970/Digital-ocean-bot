package network

import (
	"errors"
	"os"
	"path/filepath"
)

// BrowserSessionRoot is trusted process configuration, never a request field.
// Instances may isolate browser artifacts; test runners use a private directory.
func BrowserSessionRoot() (string, error) {
	root := os.Getenv("DOB_BROWSER_SESSIONS_DIR")
	if root == "" {
		root = "/var/lib/digital-ocean-bot/browser-sessions"
	}
	if !filepath.IsAbs(root) || filepath.Clean(root) == "/" || filepath.Clean(root) != root {
		return "", errors.New("invalid browser session root")
	}
	return root, nil
}
