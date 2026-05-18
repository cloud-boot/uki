package index

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
)

func TestPlatformList(t *testing.T) {
	var ps PlatformList
	if err := ps.Set("linux/amd64=ref"); err != nil {
		t.Fatal(err)
	}
	if err := ps.Set("malformed"); err == nil {
		t.Fatal("expected error for malformed pair")
	}
	if len(ps) != 1 || ps[0].Platform != "linux/amd64" || ps[0].Ref != "ref" {
		t.Errorf("unexpected list state: %+v", ps)
	}
	if s := ps.String(); s == "" {
		t.Error("String() returned empty")
	}
	if ps.Type() == "" {
		t.Error("Type() returned empty")
	}
}

func TestRun_NoPlatforms(t *testing.T) {
	if err := Run(Opts{OutRef: "ref"}); err == nil {
		t.Fatal("expected error")
	}
}

func TestRun_BadPlatform(t *testing.T) {
	o := Opts{OutRef: "out/repo:tag"}
	_ = o.Platforms.Set("bad=ref")
	if err := Run(o); err == nil {
		t.Fatal("expected platform error")
	}
}

func TestRun_BadOutRef(t *testing.T) {
	o := Opts{OutRef: "noslash"}
	_ = o.Platforms.Set("linux/amd64=ref/repo@sha256:abc")
	if err := Run(o); err == nil {
		t.Fatal("expected ParseRef error")
	}
}

func TestRun_DescribeFails(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(404)
	}))
	defer srv.Close()
	u, _ := url.Parse(srv.URL)
	o := Opts{OutRef: u.Host + "/r:out"}
	_ = o.Platforms.Set("linux/amd64=" + u.Host + "/r:t")
	if err := Run(o); err == nil {
		t.Fatal("expected describe error")
	}
}

func TestRun_End2End(t *testing.T) {
	manifestRaw := []byte(`{"mediaType":"` + ocispec.MediaTypeImageManifest + `","schemaVersion":2}`)
	mux := http.NewServeMux()
	mux.HandleFunc("/v2/r/manifests/", func(w http.ResponseWriter, req *http.Request) {
		switch req.Method {
		case "GET":
			w.Header().Set("Content-Type", ocispec.MediaTypeImageManifest)
			w.Write(manifestRaw)
		case "PUT":
			w.WriteHeader(201)
		}
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	u, _ := url.Parse(srv.URL)
	o := Opts{OutRef: u.Host + "/r:multi"}
	_ = o.Platforms.Set(fmt.Sprintf("linux/amd64=%s/r:amd64", u.Host))
	_ = o.Platforms.Set(fmt.Sprintf("linux/arm64=%s/r:arm64", u.Host))
	if err := Run(o); err != nil {
		t.Fatal(err)
	}
}

func TestRun_PushIndexFails(t *testing.T) {
	manifestRaw := []byte(`{"mediaType":"` + ocispec.MediaTypeImageManifest + `","schemaVersion":2}`)
	mux := http.NewServeMux()
	mux.HandleFunc("/v2/r/manifests/", func(w http.ResponseWriter, req *http.Request) {
		switch req.Method {
		case "GET":
			w.Header().Set("Content-Type", ocispec.MediaTypeImageManifest)
			w.Write(manifestRaw)
		case "PUT":
			w.WriteHeader(400)
		}
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	u, _ := url.Parse(srv.URL)
	o := Opts{OutRef: u.Host + "/r:out"}
	_ = o.Platforms.Set(fmt.Sprintf("linux/amd64=%s/r:t", u.Host))
	if err := Run(o); err == nil {
		t.Fatal("expected PushIndex error")
	}
}
