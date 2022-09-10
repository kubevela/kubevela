/*
Copyright 2021 The KubeVela Authors.

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

package http

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"io"
	"net/http"
	"time"

	"cuelang.org/go/cue"
	"github.com/pkg/errors"

	"github.com/kubevela/workflow/pkg/cue/model/value"

	"github.com/oam-dev/kubevela/pkg/builtin/registry"
)

func init() {
	registry.RegisterRunner("http", newHTTPCmd)
}

// HTTPCmd provides methods for http task
type HTTPCmd struct{}

func newHTTPCmd(_ cue.Value) (registry.Runner, error) {
	return &HTTPCmd{}, nil
}

// Run exec the actual http logic, and res represent the result of http task
func (c *HTTPCmd) Run(meta *registry.Meta) (res interface{}, err error) {
	var header, trailer http.Header
	var (
		method = meta.String("method")
		u      = meta.String("url")
	)
	var (
		r      io.Reader
		client = &http.Client{
			Transport: http.DefaultTransport,
			Timeout:   time.Second * 3,
		}
	)
	if obj := meta.Obj.LookupPath(value.FieldPath("request")); obj.Exists() {
		if v := obj.LookupPath(value.FieldPath("body")); v.Exists() {
			r, err = v.Reader()
			if err != nil {
				return nil, err
			}
		}
		if header, err = parseHeaders(obj, "header"); err != nil {
			return nil, err
		}
		if trailer, err = parseHeaders(obj, "trailer"); err != nil {
			return nil, err
		}
	}
	if header == nil {
		header = map[string][]string{}
		header.Set("Content-Type", "application/json")
	}
	if meta.Err != nil {
		return nil, meta.Err
	}

	req, err := http.NewRequestWithContext(context.Background(), method, u, r)
	if err != nil {
		return nil, err
	}
	req.Header = header
	req.Trailer = trailer

	if tlsConfig := meta.Obj.LookupPath(value.FieldPath("tls_config")); tlsConfig.Exists() {
		tr := &http.Transport{
			TLSClientConfig: &tls.Config{
				NextProtos: []string{"http/1.1"},
			},
		}
		ca := tlsConfig.LookupPath(value.FieldPath("ca"))
		if caCrt, err := ca.String(); err != nil {
			return nil, errors.WithMessage(err, "parse ca")
		} else {
			pool := x509.NewCertPool()
			pool.AppendCertsFromPEM([]byte(caCrt))
			tr.TLSClientConfig.RootCAs = pool
		}

		cert := tlsConfig.LookupPath(value.FieldPath("client_crt"))
		key := tlsConfig.LookupPath(value.FieldPath("client_key"))
		if cert.Exists() && key.Exists() {
			crtData, err := cert.String()
			if err != nil {
				return nil, err
			}
			keyData, err := key.String()
			if err != nil {
				return nil, err
			}
			cliCrt, err := tls.X509KeyPair([]byte(crtData), []byte(keyData))
			if err != nil {
				return nil, errors.WithMessage(err, "parse client keypair")
			}
			tr.TLSClientConfig.Certificates = []tls.Certificate{cliCrt}
		}

		client.Transport = tr
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	//nolint:errcheck
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	// parse response body and headers
	return map[string]interface{}{
		"body":       string(b),
		"header":     resp.Header,
		"trailer":    resp.Trailer,
		"statusCode": resp.StatusCode,
	}, err
}

func parseHeaders(obj cue.Value, label string) (http.Header, error) {
	m := obj.LookupPath(value.FieldPath(label))
	if !m.Exists() {
		return nil, nil
	}
	iter, err := m.Fields()
	if err != nil {
		return nil, err
	}
	h := http.Header{}
	for iter.Next() {
		str, err := iter.Value().String()
		if err != nil {
			return nil, err
		}
		h.Add(iter.Label(), str)
	}
	return h, nil
}
