package clictlplanefuncs

import (
	"errors"
	"strings"
	"testing"
)

func TestRestResult_Success(t *testing.T) {
	body := []byte(`{"success":true,"id":"v1"}`)
	got, err := restResult(body, 200, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(got) != string(body) {
		t.Fatalf("body = %q, want %q", got, body)
	}
}

func TestRestResult_TransportError(t *testing.T) {
	want := errors.New("dial tcp: connection refused")
	_, err := restResult(nil, 0, want)
	if err != want {
		t.Fatalf("err = %v, want %v", err, want)
	}
}

func TestRestResult_Non2xxWithJSONError(t *testing.T) {
	body := []byte(`{"success":false,"error":"vdev not found"}`)
	_, err := restResult(body, 404, nil)
	if err == nil {
		t.Fatal("expected error for 404")
	}
	if !strings.Contains(err.Error(), "404") || !strings.Contains(err.Error(), "vdev not found") {
		t.Fatalf("err = %q, want it to mention status and message", err.Error())
	}
}

func TestRestResult_Non2xxNonJSONBody(t *testing.T) {
	body := []byte(`internal server error`)
	_, err := restResult(body, 500, nil)
	if err == nil {
		t.Fatal("expected error for 500")
	}
	if !strings.Contains(err.Error(), "500") || !strings.Contains(err.Error(), "internal server error") {
		t.Fatalf("err = %q, want it to include status and raw body", err.Error())
	}
}
