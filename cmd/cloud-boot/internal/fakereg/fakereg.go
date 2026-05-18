// Package fakereg is a tiny in-process OCI registry used by the
// cmd/cloud-boot push subcommand tests. It is compiled into the binary —
// not gated by _test.go — so multiple test packages can import it without
// having to copy the implementation.
package fakereg

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
)

// Registry captures uploads and serves HEAD/GET against in-memory state.
// Only the surface area exercised by the push subcommands is implemented.
//
// Manifest holds the most-recent manifest PUT'd (kept for tests that only
// care that "something" was pushed). Manifests tracks every PUT keyed by
// URL path, so tests that push multiple tags (e.g. push multi) can verify
// each independently.
type Registry struct {
	Server    *httptest.Server
	Blobs     map[string][]byte
	Manifest  []byte
	Manifests map[string][]byte
}

// Start spins up a registry on a free port and registers cleanup with t.
func Start(t *testing.T) *Registry {
	t.Helper()
	r := &Registry{
		Blobs:     map[string][]byte{},
		Manifests: map[string][]byte{},
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/v2/r/blobs/", func(w http.ResponseWriter, req *http.Request) {
		d := strings.TrimPrefix(req.URL.Path, "/v2/r/blobs/")
		switch req.Method {
		case "HEAD":
			if _, ok := r.Blobs[d]; ok {
				w.WriteHeader(200)
				return
			}
			w.WriteHeader(404)
		}
	})
	mux.HandleFunc("/v2/r/blobs/uploads/", func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Location", "/upload/abc")
		w.WriteHeader(202)
	})
	mux.HandleFunc("/upload/abc", func(w http.ResponseWriter, req *http.Request) {
		body, _ := io.ReadAll(req.Body)
		d := req.URL.Query().Get("digest")
		r.Blobs[d] = body
		w.WriteHeader(201)
	})
	mux.HandleFunc("/v2/r/manifests/", func(w http.ResponseWriter, req *http.Request) {
		switch req.Method {
		case "PUT":
			body, _ := io.ReadAll(req.Body)
			r.Manifest = body
			r.Manifests[req.URL.Path] = body
			w.WriteHeader(201)
		case "GET":
			body := r.Manifests[req.URL.Path]
			if body == nil {
				// Fallback to the last-pushed manifest so tests that
				// expect the legacy behaviour (single Manifest field)
				// continue to work even when querying an unknown tag.
				body = r.Manifest
			}
			w.Header().Set("Content-Type", ocispec.MediaTypeImageManifest)
			w.Write(body)
		case "HEAD":
			w.WriteHeader(200)
		}
	})
	r.Server = httptest.NewServer(mux)
	t.Cleanup(r.Server.Close)
	return r
}

// RepoRef returns "host:port/r:<tag>" pointing at the fake registry.
func (r *Registry) RepoRef(t *testing.T, tag string) string {
	t.Helper()
	u, _ := url.Parse(r.Server.URL)
	return u.Host + "/r:" + tag
}

// StatefulFailReg returns a registry whose Nth blob upload POST fails with
// 500. Useful for testing per-stage push failures (config, kernel, initrd,
// modules, cmdline, …).
func StatefulFailReg(t *testing.T, failOn int) *httptest.Server {
	t.Helper()
	calls := 0
	mux := http.NewServeMux()
	mux.HandleFunc("/v2/r/blobs/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(404)
	})
	mux.HandleFunc("/v2/r/blobs/uploads/", func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == failOn {
			w.WriteHeader(500)
			return
		}
		w.Header().Set("Location", "/upload/abc")
		w.WriteHeader(202)
	})
	mux.HandleFunc("/upload/abc", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(201)
	})
	mux.HandleFunc("/v2/r/manifests/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(201)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}
