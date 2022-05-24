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
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"

	"github.com/form3tech-oss/jwt-go"
)

// GetTokenSubject extract the subject field from the jwt token
func GetTokenSubject(token string) (string, error) {
	claims, sub := jwt.MapClaims{}, ""
	_, err := jwt.ParseWithClaims(token, claims, nil)
	if len(claims) > 0 {
		sub, _ = claims["sub"].(string)
	}
	return sub, err
}

// GetCertificateSubject extract Subject from Certificate
func GetCertificateSubject(certificate []byte) (*pkix.Name, error) {
	if len(certificate) == 0 {
		return nil, nil
	}
	blk, _ := pem.Decode(certificate)
	if blk == nil {
		return nil, nil
	}
	cert, err := x509.ParseCertificate(blk.Bytes)
	if err != nil {
		return nil, err
	}
	return &cert.Subject, nil
}
