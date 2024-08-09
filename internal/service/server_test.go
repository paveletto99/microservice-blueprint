/*===========================================================================*\

\*===========================================================================*/

package service

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"io"
	"net/http"
	"os"
	"path"
	"testing"

	"github.com/quic-go/quic-go/http3"
)

// func TestPoboRunner(t *testing.T) {
// 	// x := NewPobo()
// 	if x == nil {
// 		t.Errorf("Failure")
// 	}
// }

func TestQuicClient(t *testing.T) {

	// TODO mock running server

	roundTripper := &http3.RoundTripper{
		TLSClientConfig: &tls.Config{
			RootCAs:            GetRootCA(),
			InsecureSkipVerify: true,
		}, // set a TLS client config, if desired
	}
	defer roundTripper.Close()
	client := &http.Client{
		Transport: roundTripper,
	}

	r, e := client.Get("https://127.0.0.1:44591/")
	if e != nil {
		t.Error("FAIL")
	}

	for k, v := range r.Header {
		t.Log(k, v)
	}

	defer r.Body.Close()
	resBody, _ := io.ReadAll(r.Body)
	response := string(resBody)

	t.Log(response)
}

// GetRootCA returns an x509.CertPool containing the CA certificate
func GetRootCA() *x509.CertPool {
	caCertPath := path.Join("../../tools/certs", "certificate.pem")
	caCertRaw, err := os.ReadFile(caCertPath)
	if err != nil {
		panic(err)
	}
	p, _ := pem.Decode(caCertRaw)
	if p.Type != "CERTIFICATE" {
		panic("expected a certificate")
	}
	caCert, err := x509.ParseCertificate(p.Bytes)
	if err != nil {
		panic(err)
	}
	certPool := x509.NewCertPool()
	certPool.AddCert(caCert)
	return certPool
}
