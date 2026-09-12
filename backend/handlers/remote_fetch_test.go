package handlers

import (
	"bytes"
	"context"
	"errors"
	"net"
	"strings"
	"testing"
)

func TestIsForbiddenRemoteIP(t *testing.T) {
	for _, raw := range []string{
		"10.0.0.1", "127.0.0.1", "169.254.1.1", "100.64.0.1", "192.0.2.1",
		"198.18.0.1", "198.51.100.1", "203.0.113.1", "224.0.0.1", "0.0.0.0",
		"::", "::1", "fc00::1", "fe80::1", "ff02::1", "2001:db8::1",
	} {
		if !isForbiddenRemoteIP(net.ParseIP(raw)) {
			t.Errorf("isForbiddenRemoteIP(%q) = false", raw)
		}
	}
	if isForbiddenRemoteIP(net.ParseIP("93.184.216.34")) {
		t.Error("public address was rejected")
	}
}

func TestValidateRemoteFetchURLRejectsUnsafeTargets(t *testing.T) {
	oldLookup := remoteFetchLookupIP
	t.Cleanup(func() { remoteFetchLookupIP = oldLookup })
	remoteFetchLookupIP = func(_ context.Context, host string) ([]net.IP, error) {
		switch host {
		case "private.test":
			return []net.IP{net.ParseIP("192.168.1.2")}, nil
		case "public.test":
			return []net.IP{net.ParseIP("93.184.216.34")}, nil
		default:
			return nil, errors.New("not found")
		}
	}

	for _, raw := range []string{
		"ftp://public.test/list.txt", "http://user:pass@public.test/list.txt",
		"http://public.test/list.txt#fragment", "http://public.test:bad/list.txt",
		"http://private.test/list.txt",
	} {
		if _, _, err := validateRemoteFetchURL(context.Background(), raw); !errors.Is(err, errInvalidRemoteURL) {
			t.Errorf("validateRemoteFetchURL(%q) error = %v, want invalid URL", raw, err)
		}
	}

	u, endpoint, err := validateRemoteFetchURL(context.Background(), "https://public.test/list.txt")
	if err != nil {
		t.Fatalf("valid URL rejected: %v", err)
	}
	if u.Scheme != "https" || endpoint.host != "public.test" || len(endpoint.ips) != 1 {
		t.Fatalf("unexpected validated target: URL=%v endpoint=%+v", u, endpoint)
	}
}

func TestReadRemoteBlocklistBodyDoesNotTruncate(t *testing.T) {
	tooLarge := bytes.Repeat([]byte("x"), maxRemoteBlocklistBytes+1)
	if _, err := readRemoteBlocklistBody(bytes.NewReader(tooLarge), -1); !errors.Is(err, errRemoteResponseTooLarge) {
		t.Fatalf("actual oversized body error = %v, want too large", err)
	}
	if _, err := readRemoteBlocklistBody(strings.NewReader("small"), maxRemoteBlocklistBytes+1); !errors.Is(err, errRemoteResponseTooLarge) {
		t.Fatalf("declared oversized body error = %v, want too large", err)
	}
	got, err := readRemoteBlocklistBody(bytes.NewReader([]byte("small")), int64(len("small")))
	if err != nil || string(got) != "small" {
		t.Fatalf("bounded body = %q, error = %v", got, err)
	}
}
