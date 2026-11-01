// Command Gale is a download manager for Windows: several connections per
// file, resumable downloads, a queue, and a browser extension that hands
// downloads over.
package main

import (
	"os"

	"gale/internal/app"
)

func main() {
	os.Exit(app.Main())
}
