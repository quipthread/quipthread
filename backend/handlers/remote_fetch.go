package handlers

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	maxRemoteBlocklistBytes = 1 << 20
	remoteFetchTimeout      = 10 * time.Second
	maxRemoteRedirects      = 3
)

var errInvalidRemoteURL = errors.New("invalid remote URL")
var errRemoteResponseTooLarge = errors.New("remote response too large")

// remoteFetchLookupIP is a seam for focused tests. Production requests use
// the system resolver, but the result is captured in the request context and
// the transport dials only those validated addresses.
var remoteFetchLookupIP = lookupRemoteFetchIPs

type remoteFetchEndpoint struct {
	host string
	ips  []net.IP
}

type remoteFetchEndpointKey struct{}

func lookupRemoteFetchIPs(ctx context.Context, host string) ([]net.IP, error) {
	if ip := net.ParseIP(host); ip != nil {
		return []net.IP{ip}, nil
	}
	addrs, err := net.DefaultResolver.LookupIP(ctx, "ip", host)
	if err != nil {
		return nil, err
	}
	return addrs, nil
}

func validateRemoteFetchURL(ctx context.Context, raw string) (*url.URL, remoteFetchEndpoint, error) {
	u, err := url.Parse(raw)
	if err != nil || u == nil || !u.IsAbs() || u.Host == "" || u.Opaque != "" {
		return nil, remoteFetchEndpoint{}, errInvalidRemoteURL
	}
	scheme := strings.ToLower(u.Scheme)
	if scheme != "http" && scheme != "https" {
		return nil, remoteFetchEndpoint{}, errInvalidRemoteURL
	}
	if u.User != nil || u.Fragment != "" || u.Hostname() == "" {
		return nil, remoteFetchEndpoint{}, errInvalidRemoteURL
	}
	// url.Parse rejects most malformed ports, but an empty explicit port is
	// accepted by some Go versions and is not a valid import target.
	if strings.HasSuffix(u.Host, ":") {
		return nil, remoteFetchEndpoint{}, errInvalidRemoteURL
	}
	port := u.Port()
	if port != "" {
		p, parseErr := strconv.Atoi(port)
		if parseErr != nil || p < 1 || p > 65535 {
			return nil, remoteFetchEndpoint{}, errInvalidRemoteURL
		}
	}
	host := strings.ToLower(u.Hostname())
	ips, err := remoteFetchLookupIP(ctx, host)
	if err != nil || len(ips) == 0 {
		return nil, remoteFetchEndpoint{}, errInvalidRemoteURL
	}
	validated := make([]net.IP, 0, len(ips))
	for _, ip := range ips {
		if ip == nil || isForbiddenRemoteIP(ip) {
			return nil, remoteFetchEndpoint{}, errInvalidRemoteURL
		}
		validated = append(validated, append(net.IP(nil), ip...))
	}
	u.Scheme = scheme
	return u, remoteFetchEndpoint{host: host, ips: validated}, nil
}

func isForbiddenRemoteIP(ip net.IP) bool {
	if ip == nil || ip.IsUnspecified() || ip.IsLoopback() ||
		ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsMulticast() || ip.IsPrivate() {
		return true
	}

	if v4 := ip.To4(); v4 != nil {
		// RFC 6598 CGNAT and IPv4 special-use/reserved ranges. The explicit
		// list supplements net.IP.IsPrivate, which intentionally covers only
		// RFC 1918 and IPv6 ULA ranges.
		for _, cidr := range []string{
			"0.0.0.0/8", "100.64.0.0/10", "192.0.0.0/24", "192.0.2.0/24",
			"192.31.196.0/24", "192.52.193.0/24", "192.88.99.0/24",
			"198.18.0.0/15", "198.51.100.0/24", "203.0.113.0/24", "240.0.0.0/4",
		} {
			if _, network, err := net.ParseCIDR(cidr); err == nil && network.Contains(v4) {
				return true
			}
		}
		return false
	}

	// IPv6 documentation, benchmarking, discard, and protocol-assignment
	// ranges are not routable application destinations and are treated as
	// special-use along with the ranges covered by the IP methods above.
	for _, cidr := range []string{
		"100::/64", "2001:1::/32", "2001:2::/48", "2001:10::/28", "2001:20::/28",
		"2001:db8::/32", "3fff::/20", "5f00::/16",
	} {
		if _, network, err := net.ParseCIDR(cidr); err == nil && network.Contains(ip) {
			return true
		}
	}
	return false
}

func newRemoteFetchClient() *http.Client {
	transport := &http.Transport{
		// This client is used only for caller-supplied blocklist URLs. It must
		// never inherit HTTP(S)_PROXY or an ambient proxy configuration.
		Proxy: nil,
		DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			endpoint, ok := ctx.Value(remoteFetchEndpointKey{}).(remoteFetchEndpoint)
			if !ok || len(endpoint.ips) == 0 {
				return nil, errors.New("remote fetch target was not validated")
			}
			host, port, err := net.SplitHostPort(address)
			if err != nil || !strings.EqualFold(host, endpoint.host) {
				return nil, errors.New("remote fetch target changed")
			}
			var selected net.IP
			for _, ip := range endpoint.ips {
				if network == "tcp4" && ip.To4() == nil || network == "tcp6" && ip.To4() != nil {
					continue
				}
				selected = ip
				break
			}
			if selected == nil {
				return nil, errors.New("no validated address for network")
			}
			dialer := net.Dialer{Timeout: remoteFetchTimeout, KeepAlive: 30 * time.Second}
			return dialer.DialContext(ctx, network, net.JoinHostPort(selected.String(), port))
		},
		TLSHandshakeTimeout:   remoteFetchTimeout,
		ResponseHeaderTimeout: remoteFetchTimeout,
		ExpectContinueTimeout: 1 * time.Second,
	}
	client := &http.Client{
		Transport: transport,
		Timeout:   remoteFetchTimeout,
	}
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) >= maxRemoteRedirects {
			return errors.New("too many redirects")
		}
		_, endpoint, err := validateRemoteFetchURL(req.Context(), req.URL.String())
		if err != nil {
			return err
		}
		// CheckRedirect receives a pointer to the request that will be sent.
		// Replace it in place so DialContext sees the newly validated host/IPs.
		*req = *req.WithContext(context.WithValue(req.Context(), remoteFetchEndpointKey{}, endpoint))
		return nil
	}
	return client
}

func fetchUntrustedBlocklistURL(parent context.Context, rawURL string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(parent, remoteFetchTimeout)
	defer cancel()
	u, endpoint, err := validateRemoteFetchURL(ctx, rawURL)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, errInvalidRemoteURL
	}
	req = req.WithContext(context.WithValue(req.Context(), remoteFetchEndpointKey{}, endpoint))
	resp, err := newRemoteFetchClient().Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close() //nolint:errcheck // response is closed after bounded read
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("remote response status %d", resp.StatusCode)
	}
	raw, err := readRemoteBlocklistBody(resp.Body, resp.ContentLength)
	if err != nil {
		return nil, err
	}
	return raw, nil
}

func readRemoteBlocklistBody(body io.Reader, declaredLength int64) ([]byte, error) {
	if declaredLength > maxRemoteBlocklistBytes {
		return nil, errRemoteResponseTooLarge
	}
	raw, err := io.ReadAll(io.LimitReader(body, maxRemoteBlocklistBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(raw)) > maxRemoteBlocklistBytes {
		return nil, errRemoteResponseTooLarge
	}
	return raw, nil
}
