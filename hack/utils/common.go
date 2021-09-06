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

package utils

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"io"
	"os"
	"strings"

	helm "helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chart/loader"
)

// FprintZipData converts zip binary contents to a string literal.
// From https://github.com/rakyll/statik/blob/master/statik.go#L313
func FprintZipData(dest *bytes.Buffer, zipData []byte) {
	for _, b := range zipData {
		if b == '\n' {
			dest.WriteString(`\n`)
			continue
		}
		if b == '\\' {
			dest.WriteString(`\\`)
			continue
		}
		if b == '"' {
			dest.WriteString(`\"`)
			continue
		}
		if (b >= 32 && b <= 126) || b == '\t' {
			dest.WriteByte(b)
			continue
		}
		fmt.Fprintf(dest, "\\x%02x", b)
	}
}

// GetChartSource is a helper to convert a filepath to a chart to a
// base64-encoded, gzipped tarball.
func GetChartSource(path string) (string, error) {
	pack := helm.NewPackage()
	packagedPath, err := pack.Run(path, nil)
	if err != nil {
		return "", err
	}
	defer os.Remove(packagedPath)
	packaged, err := os.ReadFile(packagedPath)
	if err != nil {
		return "", err
	}
	b64Encoded := bytes.NewBuffer(nil)
	enc := base64.NewEncoder(base64.StdEncoding, b64Encoded)
	_, err = io.Copy(enc, bytes.NewReader(packaged))
	if err != nil {
		return "", err
	}
	return b64Encoded.String(), nil
}

// LoadChart is a helper to turn a base64-encoded, gzipped tarball into a chart.
func LoadChart(source string) (*chart.Chart, error) {
	dec := base64.NewDecoder(base64.StdEncoding, strings.NewReader(source))
	tgz := bytes.NewBuffer(nil)
	_, err := io.Copy(tgz, dec)
	if err != nil {
		return nil, err
	}
	return loader.LoadArchive(tgz)
}
