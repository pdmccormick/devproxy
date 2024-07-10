package devproxycmd

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"time"
)

func makeCertificate() (*tls.Certificate, error) {
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}

	serialMax := new(big.Int).Lsh(big.NewInt(1), 128)
	serial, err := rand.Int(rand.Reader, serialMax)
	if err != nil {
		return nil, err
	}

	var (
		now      = time.Now()
		template = &x509.Certificate{
			SerialNumber: serial,
			Subject: pkix.Name{
				Organization:       []string{"go.pdmccormick.com"},
				OrganizationalUnit: []string{"devproxy"},
				CommonName:         "devproxy",
			},

			NotBefore: now,
			NotAfter:  now.AddDate(0, 3, 0),

			KeyUsage:    x509.KeyUsageCertSign | x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
			ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},

			BasicConstraintsValid: true,
			IsCA:                  true,
			MaxPathLenZero:        true,
		}
	)

	certDer, err := x509.CreateCertificate(rand.Reader, template, template, priv.Public(), priv)
	if err != nil {
		return nil, err
	}

	cert, err := x509.ParseCertificate(certDer)
	if err != nil {
		return nil, err
	}

	var tlsCert = tls.Certificate{
		Certificate: [][]byte{certDer},
		PrivateKey:  priv,
		Leaf:        cert,
	}

	return &tlsCert, nil
}
