package agent

import (
	"os"
	"strings"
)

func readStringFile(path string) string {
	content, _ := readStringFileOK(path)
	return content
}

func readStringFileOK(path string) (string, bool) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", false
	}
	return strings.TrimSpace(string(b)), true
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
