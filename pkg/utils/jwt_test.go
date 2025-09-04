/*
Copyright 2022 The KubeVela Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

	http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package utils

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"testing"
	"time"

	"github.com/form3tech-oss/jwt-go"
	"github.com/stretchr/testify/require"
)

func TestGetTokenSubject(t *testing.T) {
	t.Parallel()

	tokenWithSub := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{"sub": "test-user"})
	tokenWithSubStr, err := tokenWithSub.SignedString([]byte("secret"))
	require.NoError(t, err)

	tokenWithoutSub := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{"user": "test-user"})
	tokenWithoutSubStr, err := tokenWithoutSub.SignedString([]byte("secret"))
	require.NoError(t, err)

	testCases := []struct {
		name        string
		token       string
		expectedSub string
		expectErr   bool
	}{
		{
			name:        "valid token with sub",
			token:       tokenWithSubStr,
			expectedSub: "test-user",
			// An error is returned because the signature cannot be verified without a key func,
			// but the subject should still be extracted.
			expectErr: true,
		},
		{
			name:        "valid token without sub",
			token:       tokenWithoutSubStr,
			expectedSub: "",
			expectErr:   true,
		},
		{
			name:        "malformed token",
			token:       "a.b.c",
			expectedSub: "",
			expectErr:   true,
		},
		{
			name:        "empty token",
			token:       "",
			expectedSub: "",
			expectErr:   true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			sub, err := GetTokenSubject(tc.token)
			if tc.expectErr {
				require.Error(t, err)
			}
			require.Equal(t, tc.expectedSub, sub)
		})
	}
}

func generateTestCert(t *testing.T, subject pkix.Name) []byte {
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	template := x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               subject,
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}

	derBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)
	require.NoError(t, err)

	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: derBytes})
	return pemBytes
}

func TestGetCertificateSubject(t *testing.T) {
	t.Parallel()
	subject := pkix.Name{CommonName: "test.example.com"}
	certPEM := generateTestCert(t, subject)

	testCases := []struct {
		name            string
		certBytes       []byte
		expectedSubject *pkix.Name
		expectErr       bool
	}{
		{
			name:            "valid cert",
			certBytes:       certPEM,
			expectedSubject: &subject,
			expectErr:       false,
		},
		{
			name:            "empty bytes",
			certBytes:       []byte{},
			expectedSubject: nil,
			expectErr:       false,
		},
		{
			name:            "not a pem block",
			certBytes:       []byte("not pem"),
			expectedSubject: nil,
			expectErr:       false,
		},
		{
			name:            "invalid cert bytes in pem",
			certBytes:       pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: []byte("invalid")}),
			expectedSubject: nil,
			expectErr:       true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			s, err := GetCertificateSubject(tc.certBytes)
			if tc.expectErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				if tc.expectedSubject == nil {
					require.Nil(t, s)
				} else {
					require.NotNil(t, s)
					require.Equal(t, tc.expectedSubject.String(), s.String())
				}
			}
		})
	}
}
