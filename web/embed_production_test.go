//go:build production

package web

import (
	"io/fs"
	"strings"
	"testing"
)

func TestProductionAssetsContainApplicationShell(t *testing.T) {
	index, err := fs.ReadFile(Assets(), "index.html")
	if err != nil {
		t.Fatalf("read embedded index: %v", err)
	}
	if !strings.Contains(string(index), "<title>LogLeopard</title>") {
		t.Fatal("embedded index does not contain the LogLeopard application shell")
	}
}
