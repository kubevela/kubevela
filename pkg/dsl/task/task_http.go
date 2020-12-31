package task

import (
	"context"
	"io"
	"io/ioutil"
	"net/http"

	"cuelang.org/go/cue"
)

func init() {
	Register("http", newHTTPCmd)
}

// HTTPCmd provides methods for http task
type HTTPCmd struct {
	*http.Client
}

func newHTTPCmd(v cue.Value) (Runner, error) {
	client := http.DefaultClient
	return &HTTPCmd{client}, nil
}

// Run exec the actual http logic
func (c *HTTPCmd) Run(ctx *Context) (res interface{}, err error) {
	var header, trailer http.Header
	var (
		method = ctx.String("method")
		u      = ctx.String("url")
	)
	var r io.Reader
	if obj := ctx.Obj.Lookup("request"); obj.Exists() {
		if v := obj.Lookup("body"); v.Exists() {
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
		header.Set("Content-Type", "application/json")
	}
	if ctx.Err != nil {
		return nil, ctx.Err
	}

	req, err := http.NewRequestWithContext(context.Background(), method, u, r)
	if err != nil {
		return nil, err
	}
	req.Header = header
	req.Trailer = trailer

	resp, err := c.Client.Do(req)
	if err != nil {
		return nil, err
	}
	//nolint:errcheck
	defer resp.Body.Close()
	b, err := ioutil.ReadAll(resp.Body)
	// parse response body and headers
	return map[string]interface{}{
		"body":    string(b),
		"header":  resp.Header,
		"trailer": resp.Trailer,
	}, err
}

func parseHeaders(obj cue.Value, label string) (http.Header, error) {
	m := obj.Lookup(label)
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
