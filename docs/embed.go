package docs

import _ "embed"

// userGuide holds the Markdown manual so it is available in the binary.
//
//go:embed user-guide.md
var userGuide string

// UserGuide returns the embedded user manual.
func UserGuide() string {
	return userGuide
}
