package utils

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"io"
	"io/ioutil"
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
	packaged, err := ioutil.ReadFile(packagedPath)
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
