// Package rtfake turns an http.Handler into an http.RoundTripper so that real
// provider SDK clients can talk to an in-process fake without a socket.
package rtfake

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
)

// Transport serves every request directly from h. No listener is opened.
func Transport(h http.Handler) http.RoundTripper {
	return roundTripper{h}
}

// Client is a convenience wrapper around Transport.
func Client(h http.Handler) *http.Client {
	return &http.Client{Transport: Transport(h)}
}

type roundTripper struct{ h http.Handler }

func (rt roundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if err := req.Context().Err(); err != nil {
		return nil, err
	}
	// Handlers expect a server-side request; RequestURI is only set there.
	sreq := req.Clone(req.Context())
	sreq.RequestURI = req.URL.RequestURI()
	if sreq.Host == "" {
		sreq.Host = req.URL.Host
	}
	rec := httptest.NewRecorder()
	rt.h.ServeHTTP(rec, sreq)
	res := rec.Result()
	res.Request = req
	return res, nil
}

// JSON writes v as a JSON body with the given status and content type.
func JSON(w http.ResponseWriter, status int, contentType string, v any) {
	if contentType == "" {
		contentType = "application/json"
	}
	w.Header().Set("Content-Type", contentType)
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// ByHeader dispatches on the value of a request header, which is how the AWS
// JSON protocol selects an operation (X-Amz-Target). Unknown values go to
// notFound.
func ByHeader(name string, routes map[string]http.HandlerFunc, notFound http.HandlerFunc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if h, ok := routes[r.Header.Get(name)]; ok {
			h(w, r)
			return
		}
		notFound(w, r)
	})
}
