package client

import (
	"bytes"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

var errAPIIdentityMismatch = errors.New("api_identity_mismatch")

// Transport construction is internal; command-line flags and server replies
// cannot replace release trust anchors or select another payment destination.
func pinnedHTTP(base string, pins []string) (*http.Client, error) {
	u, e := url.Parse(base)
	if e != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Path != "" {
		return nil, ErrInvalid
	}
	t := &http.Transport{Proxy: nil, DisableCompression: true, DialContext: (&net.Dialer{Timeout: 5 * time.Second}).DialContext, TLSHandshakeTimeout: 5 * time.Second, ResponseHeaderTimeout: 30 * time.Second, MaxResponseHeaderBytes: ResponseLimit}
	t.TLSClientConfig = &tls.Config{MinVersion: tls.VersionTLS12}
	if len(pins) > 0 {
		for _, p := range pins {
			if !hexDigest.MatchString(p) {
				return nil, ErrInvalid
			}
		}
		t.TLSClientConfig.VerifyConnection = func(cs tls.ConnectionState) error {
			if len(cs.PeerCertificates) == 0 {
				return errAPIIdentityMismatch
			}
			h := sha256.Sum256(cs.PeerCertificates[0].RawSubjectPublicKeyInfo)
			for _, p := range pins {
				if hex.EncodeToString(h[:]) == p {
					return nil
				}
			}
			return errAPIIdentityMismatch
		}
	}
	return &http.Client{Transport: &onlyOrigin{u.Scheme + "://" + u.Host, t}, Timeout: 30 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return errors.New("redirect_refused") }}, nil
}

type onlyOrigin struct {
	origin string
	next   http.RoundTripper
}

func (t *onlyOrigin) RoundTrip(r *http.Request) (*http.Response, error) {
	if r.URL.Scheme+"://"+r.URL.Host != t.origin || r.URL.User != nil {
		return nil, ErrInvalid
	}
	return t.next.RoundTrip(r)
}
func readResponse(resp *http.Response) ([]byte, error) {
	defer resp.Body.Close()
	if resp.ContentLength > ResponseLimit || resp.Header.Get("Content-Encoding") != "" {
		return nil, ErrInvalid
	}
	ct := resp.Header.Get("Content-Type")
	if !strings.HasPrefix(ct, "application/json") {
		return nil, ErrInvalid
	}
	b, e := io.ReadAll(io.LimitReader(resp.Body, ResponseLimit+1))
	if e != nil || len(b) > ResponseLimit {
		return nil, ErrInvalid
	}
	if checkJSON(b, ResponseLimit) != nil {
		return nil, ErrInvalid
	}
	return b, nil
}
func jsonBody(b []byte) io.Reader { return bytes.NewReader(b) }
