package rtfake

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestTransportServesWithoutSocket(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		JSON(w, http.StatusCreated, "", map[string]string{
			"path": r.RequestURI, "body": string(b), "host": r.Host,
		})
	})
	c := Client(h)

	res, err := c.Post("http://fake.invalid/v1/x?y=1", "text/plain", strings.NewReader("hi"))
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("status = %d", res.StatusCode)
	}
	b, _ := io.ReadAll(res.Body)
	got := string(b)
	for _, want := range []string{`"/v1/x?y=1"`, `"hi"`, `"fake.invalid"`} {
		if !strings.Contains(got, want) {
			t.Errorf("body %s missing %s", got, want)
		}
	}
}

func TestTransportHonoursCancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "http://fake.invalid/", nil)
	if _, err := Client(http.NotFoundHandler()).Do(req); err == nil {
		t.Fatal("expected error for cancelled context")
	}
}

func TestByHeader(t *testing.T) {
	h := ByHeader("X-Amz-Target", map[string]http.HandlerFunc{
		"svc.Get": func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) },
	}, func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNotImplemented) })
	c := Client(h)

	for target, want := range map[string]int{"svc.Get": 200, "svc.Nope": 501, "": 501} {
		req, _ := http.NewRequest(http.MethodPost, "http://fake.invalid/", nil)
		req.Header.Set("X-Amz-Target", target)
		res, err := c.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		if res.StatusCode != want {
			t.Errorf("target %q: status %d, want %d", target, res.StatusCode, want)
		}
	}
}
