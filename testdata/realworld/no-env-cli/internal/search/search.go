// Package search counts pattern matches in a file.
package search

import (
	"bufio"
	"os"
	"strings"
)

// Count returns the number of lines of the file at path containing pattern.
func Count(pattern, path string) (int, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	n := 0
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if strings.Contains(sc.Text(), pattern) {
			n++
		}
	}
	return n, sc.Err()
}
