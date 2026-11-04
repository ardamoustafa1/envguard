// Command wcgrep counts matches of a pattern in files.
package main

import (
	"fmt"
	"os"

	"example.com/wcgrep/internal/config"
	"example.com/wcgrep/internal/search"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, "wcgrep:", err)
		os.Exit(2)
	}
	if len(os.Args) < 3 {
		fmt.Fprintln(os.Stderr, "usage: wcgrep PATTERN FILE...")
		os.Exit(2)
	}
	total := 0
	for _, f := range os.Args[2:] {
		n, err := search.Count(os.Args[1], f)
		if err != nil {
			fmt.Fprintln(os.Stderr, "wcgrep:", err)
			os.Exit(2)
		}
		total += n
		fmt.Fprintf(os.Stdout, "%s%s%s:%d\n", cfg.Color("1"), f, cfg.Color("0"), n)
	}
	if total == 0 {
		os.Exit(1)
	}
}
