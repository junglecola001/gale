// Command testserver serves a file (100 MB by default) to download from,
// with switches for the failures the engine must handle.
//
//	go run ./cmd/testserver -addr 127.0.0.1:8099 -size 100MB
package main

import (
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"strconv"
	"strings"

	"gale/internal/testutil"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:8099", "address to listen on")
	size := flag.String("size", "100MB", "size of the file, e.g. 100MB")
	name := flag.String("name", "file.bin", "file name the server suggests")
	noRange := flag.Bool("no-range", false, "ignore Range requests (no resuming)")
	dropAfter := flag.String("drop-after", "", "cut the first response off after this many bytes, e.g. 30MB")
	etagAfter := flag.String("etag-change-after", "", "change the file after this many bytes served, e.g. 40MB")
	etagTimes := flag.Int("etag-changes", 1, "how many times the file changes")
	rate := flag.String("rate", "", "limit the server to this many bytes per second, e.g. 2MB")
	chunked := flag.Bool("chunked", false, "send the body without a length")
	delay := flag.Duration("delay-first", 0, "hold the first response back this long, e.g. 300ms")
	flag.Parse()

	opts := testutil.Options{
		Size:            parseSize(*size),
		Name:            *name,
		NoRange:         *noRange,
		DropAfter:       parseSize(*dropAfter),
		ETagChangeAfter: parseSize(*etagAfter),
		ETagChanges:     *etagTimes,
		Rate:            parseSize(*rate),
		Chunked:         *chunked,
		DelayFirst:      *delay,
	}
	handler := testutil.NewHandler(opts)
	sum := handler.SHA256()

	page := fmt.Sprintf(`<!doctype html><meta charset="utf-8"><title>Gale test server</title>
<style>body{font:14px system-ui;margin:24px;background:#111;color:#eee}a{color:#6aa0ff}</style>
<h1>Gale test server</h1>
<p>%d bytes · sha256 <code>%s</code></p>
<p><a href="/file/%s">/file/%s</a></p>`, opts.Size, sum, *name, *name)

	mux := http.NewServeMux()
	mux.Handle("/file/", handler)
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			handler.ServeHTTP(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, page)
	})

	ln, err := net.Listen("tcp", *addr)
	if err != nil {
		log.Fatalf("listening on %s: %v", *addr, err)
	}
	fmt.Printf("sha256=%s\n", sum)
	fmt.Printf("size=%d\n", opts.Size)
	fmt.Printf("listening on http://%s/\n", ln.Addr())
	if err := http.Serve(ln, mux); err != nil {
		log.Fatal(err)
	}
}

// parseSize reads "100MB", "1500KB", "1234" and the like; "" is 0.
func parseSize(v string) int64 {
	v = strings.TrimSpace(strings.ToUpper(v))
	if v == "" {
		return 0
	}
	multipliers := []struct {
		suffix string
		value  int64
	}{
		{"KB", 1000},
		{"MB", 1000 * 1000},
		{"GB", 1000 * 1000 * 1000},
		{"KIB", 1024},
		{"MIB", 1024 * 1024},
		{"GIB", 1024 * 1024 * 1024},
		{"B", 1},
	}
	for _, m := range multipliers {
		if strings.HasSuffix(v, m.suffix) {
			number := strings.TrimSpace(strings.TrimSuffix(v, m.suffix))
			n, err := strconv.ParseFloat(number, 64)
			if err != nil {
				log.Fatalf("bad size %q", v)
			}
			return int64(n * float64(m.value))
		}
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		log.Fatalf("bad size %q", v)
	}
	return n
}
