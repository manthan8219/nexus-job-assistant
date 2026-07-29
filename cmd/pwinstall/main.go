package main

import (
	"fmt"
	"os"

	"github.com/mxschmitt/playwright-go"
)

func main() {
	fmt.Println("Installing Playwright driver + Chromium (this can take a few minutes)...")
	if err := playwright.Install(&playwright.RunOptions{
		Browsers: []string{"chromium"},
	}); err != nil {
		fmt.Fprintln(os.Stderr, "install failed:", err)
		os.Exit(1)
	}
	fmt.Println("Done.")
}
