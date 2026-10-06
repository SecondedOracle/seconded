package client

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"
)

func TestReleaseTransportCertificateRenewal(t *testing.T) {
	if os.Getenv("SECONDED_TEST_NO_NETWORK") == "1" {
		t.Skip("loopback TLS fixture disabled")
	}
	rootPub, rootKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	root := &x509.Certificate{SerialNumber: big.NewInt(1), IsCA: true, BasicConstraintsValid: true,
		KeyUsage: x509.KeyUsageCertSign, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour)}
	rootDER, err := x509.CreateCertificate(rand.Reader, root, root, rootPub, rootKey)
	if err != nil {
		t.Fatal(err)
	}
	root, err = x509.ParseCertificate(rootDER)
	if err != nil {
		t.Fatal(err)
	}
	issue := func(serial int64) tls.Certificate {
		pub, key, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			t.Fatal(err)
		}
		leaf := &x509.Certificate{SerialNumber: big.NewInt(serial), IPAddresses: []net.IP{net.ParseIP("127.0.0.1")},
			NotBefore: time.Now().Add(-time.Minute), NotAfter: time.Now().Add(time.Hour),
			KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
		der, err := x509.CreateCertificate(rand.Reader, leaf, root, pub, rootKey)
		if err != nil {
			t.Fatal(err)
		}
		return tls.Certificate{Certificate: [][]byte{der, rootDER}, PrivateKey: key}
	}
	certificates := []tls.Certificate{issue(2), issue(3)}
	index := 0
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{}`))
	}))
	srv.TLS = &tls.Config{MinVersion: tls.VersionTLS12, GetCertificate: func(*tls.ClientHelloInfo) (*tls.Certificate, error) {
		cert := certificates[index%len(certificates)]
		index++
		return &cert, nil
	}}
	srv.StartTLS()
	defer srv.Close()
	// httptest installs a default certificate; force the rotating callback.
	srv.TLS.Certificates = nil
	quorum, err := newQuorum([]EvidenceEndpoint{{URL: srv.URL, Operator: "fixture"}, {URL: "https://independent.fixture.invalid", Operator: "independent"}})
	if err != nil {
		t.Fatal(err)
	}
	c := quorum.rpcs[0].http
	transport := c.Transport.(*onlyOrigin).next.(*http.Transport)
	transport.DisableKeepAlives = true
	if transport.TLSClientConfig.InsecureSkipVerify || transport.TLSClientConfig.VerifyConnection != nil {
		t.Fatal("RPC web PKI policy bypassed or tied to a TLS key")
	}
	if resp, err := c.Get(srv.URL); err == nil {
		resp.Body.Close()
		t.Fatal("untrusted issuer accepted")
	}
	pool := x509.NewCertPool()
	pool.AddCert(root)
	transport.TLSClientConfig.RootCAs = pool
	serials := map[string]bool{}
	for i := 0; i < 2; i++ {
		resp, err := c.Get(srv.URL)
		if err != nil {
			t.Fatal("valid renewed certificate rejected", err)
		}
		serials[resp.TLS.PeerCertificates[0].SerialNumber.String()] = true
		resp.Body.Close()
	}
	if len(serials) != 2 {
		t.Fatal("renewal control did not change certificates")
	}
	transport.TLSClientConfig.ServerName = "wrong.invalid"
	if resp, err := c.Get(srv.URL); err == nil {
		resp.Body.Close()
		t.Fatal("wrong hostname accepted")
	}
}
