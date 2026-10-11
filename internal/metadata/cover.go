package metadata

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"syscall"
	"time"
)

// ErrPrivateAddress is returned when a cover URL points into a private network.
var ErrPrivateAddress = errors.New("cover URL points to a private or local address")

// cgnat is 100.64.0.0/10 — carrier-grade NAT, and Tailscale's address space.
var cgnat = netip.MustParsePrefix("100.64.0.0/10")

func publicOnly(_, address string, _ syscall.RawConn) error {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return err
	}
	ip, err := netip.ParseAddr(host)
	if err != nil {
		return err
	}
	ip = ip.Unmap()
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() ||
		ip.IsMulticast() || ip.IsUnspecified() || cgnat.Contains(ip) {
		return ErrPrivateAddress
	}
	return nil
}

// coverHTTP downloads covers from any public web server, but refuses to
// connect to private, loopback or tailnet addresses (also after redirects),
// so a pasted URL cannot be used to poke at the local network.
func coverHTTP(timeout time.Duration) *http.Client {
	dialer := &net.Dialer{Timeout: 10 * time.Second, Control: publicOnly}
	tr := &http.Transport{DialContext: dialer.DialContext, TLSHandshakeTimeout: 10 * time.Second, MaxIdleConns: 4, IdleConnTimeout: 30 * time.Second}
	return &http.Client{Timeout: timeout, Transport: tr, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 5 {
			return errors.New("too many redirects")
		}
		return nil
	}}
}

// CoverExt maps accepted image types to file extensions.
var CoverExt = map[string]string{"image/jpeg": ".jpg", "image/png": ".png", "image/webp": ".webp", "image/gif": ".gif"}

// FetchCover downloads an image (at most maxBytes) and returns it with its
// content type. Tiny images are rejected: catalogues answer "no cover" with a
// 1×1 pixel.
func (c *Client) FetchCover(ctx context.Context, rawURL string, maxBytes int64) ([]byte, string, error) {
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
		return nil, "", fmt.Errorf("not a web address: %q", rawURL)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("User-Agent", c.UserAgent)
	res, err := coverHTTP(c.HTTP.Timeout + 10*time.Second).Do(req)
	if err != nil {
		return nil, "", err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("cover download: HTTP %d", res.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(res.Body, maxBytes+1))
	if err != nil {
		return nil, "", err
	}
	if int64(len(body)) > maxBytes {
		return nil, "", fmt.Errorf("cover larger than %d MB", maxBytes>>20)
	}
	ct := http.DetectContentType(body)
	if _, ok := CoverExt[ct]; !ok {
		return nil, "", fmt.Errorf("not an image (%s)", ct)
	}
	if len(body) < 1024 {
		return nil, "", errors.New("no cover image at that address")
	}
	return body, ct, nil
}
