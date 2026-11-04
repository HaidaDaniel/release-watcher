package source_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/HaidaDaniel/release-watcher/internal/source/httpx"
)

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "testdata", name))
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return data
}

func testClient() *httpx.Client {
	return httpx.New(httpx.Config{
		UserAgent:         "release-watcher-test",
		Timeout:           3 * time.Second,
		MaxRedirects:      3,
		MaxRetries:        0,
		RateInterval:      0,
		MaxBodyBytes:      2 << 20,
		AllowPrivateHosts: true,
	})
}

func catalogHTML(start, count int) string {
	var b strings.Builder
	b.WriteString(`<div class="p-eplist">`)
	for i := start; i < start+count; i++ {
		fmt.Fprintf(&b, `<div class="p-eplist__sublist">
<a href="/n2166fe/%d/" class="p-eplist__subtitle">Chapter %d</a>
<div class="p-eplist__update">2023/08/01 22:25</div>
</div>`, i, i)
	}
	b.WriteString(`</div>`)
	return b.String()
}
