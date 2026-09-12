package middleware

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

type readCounter struct {
	reader io.Reader
	reads  int
}

func (r *readCounter) Read(p []byte) (int, error) {
	r.reads++
	return r.reader.Read(p)
}

func TestLimitRequestBodyUnknownRouteIsLazyForChunkedBody(t *testing.T) {
	body := &readCounter{reader: bytes.NewReader(bytes.Repeat([]byte("x"), int(MaxNormalBodyBytes)+1))}
	h := LimitRequestBody(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	r := httptest.NewRequest(http.MethodPost, "/api/unknown", body)
	r.ContentLength = -1
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, r)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rr.Code)
	}
	if body.reads != 0 {
		t.Fatalf("unknown route caused %d body reads, want 0", body.reads)
	}
}

func TestLimitRequestBodyRejectedRouteIsLazyForSlowBody(t *testing.T) {
	body := &readCounter{reader: bytes.NewReader(bytes.Repeat([]byte("x"), int(MaxNormalBodyBytes)+1))}
	h := LimitRequestBody(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	r := httptest.NewRequest(http.MethodPost, "/api/comments", body)
	r.ContentLength = -1
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, r)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rr.Code)
	}
	if body.reads != 0 {
		t.Fatalf("rejected route caused %d body reads, want 0", body.reads)
	}
}

func TestLimitRequestBodyKnownChunkedBodyIsCappedWhenRead(t *testing.T) {
	body := bytes.Repeat([]byte("x"), int(MaxNormalBodyBytes)+1)
	h := LimitRequestBody(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, err := io.ReadAll(r.Body)
		var maxErr *http.MaxBytesError
		if !errors.As(err, &maxErr) {
			t.Fatalf("read error = %v, want MaxBytesError", err)
		}
		w.WriteHeader(http.StatusRequestEntityTooLarge)
	}))
	r := httptest.NewRequest(http.MethodPost, "/api/comments", bytes.NewReader(body))
	r.ContentLength = -1
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, r)
	if rr.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want 413", rr.Code)
	}
}

func TestLimitRequestBodyPreservesBodyAndImportExceptions(t *testing.T) {
	const body = `{"ok":true}`
	var got string
	h := LimitRequestBody(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		got = string(data)
		w.WriteHeader(http.StatusNoContent)
	}))
	r := httptest.NewRequest(http.MethodPost, "/api/comments", bytes.NewBufferString(body))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, r)
	if rr.Code != http.StatusNoContent || got != body {
		t.Fatalf("status=%d body=%q, want 204 and restored body %q", rr.Code, got, body)
	}
	for _, path := range []string{"/api/admin/import/native", "/api/admin/import/sqlite/run", "/api/billing/webhook"} {
		limit, limited := normalBodyLimit(path)
		if limited || path == "/api/admin/import/native" && limit != maxTextImportBodyBytes || path == "/api/admin/import/sqlite/run" && limit != maxSQLiteImportBodyBytes {
			t.Fatalf("%s: limit=%d limited=%v", path, limit, limited)
		}
	}
}
