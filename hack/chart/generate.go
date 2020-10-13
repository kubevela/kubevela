package main

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/oam-dev/kubevela/hack/utils"

	"github.com/openservicemesh/osm/pkg/cli"
)

func main() {
	// Path relative to the Makefile where this is invoked.
	chartPath := filepath.Join("charts", "vela-core")
	tempChartPath, tempDir, err := fixOpenAPIV3SchemaValidationIssue(chartPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "fixing OpenAPIV3SchemaValidation issue hit an error:", err)
		os.Exit(1)
	}
	source, err := cli.GetChartSource(tempChartPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error getting chart source:", err)
		os.Exit(1)
	}
	PrintToFile(source)
	// Delete the temporary Chart path
	os.RemoveAll(tempDir)
}

func PrintToFile(data string) {
	var buffer bytes.Buffer
	buffer.WriteString(`package fake
var ChartSource = "`)
	utils.FprintZipData(&buffer, []byte(data))
	buffer.WriteString(`"`)
	_ = ioutil.WriteFile("cmd/vela/fake/chart_source.go", buffer.Bytes(), 0644)
}

// fixOpenAPIV3SchemaValidationIssue temporarily corrects spec.validation.openAPIV3Schema issue, and it would be removed
// after this issue was fixed https://github.com/oam-dev/kubevela/issues/284.
func fixOpenAPIV3SchemaValidationIssue(chartPath string) (string, string, error) {
	newDir, err := ioutil.TempDir(".", "charts")
	if err != nil {
		fmt.Fprintln(os.Stderr, "failed to crate temporary directory:", err)
		return "", "", err
	}

	err = filepath.Walk(chartPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			fmt.Fprintln(os.Stderr, "failed to list the content of", path)
			return err
		}
		targetPath := filepath.Join(newDir, path)
		if info.IsDir() {
			if err = os.MkdirAll(targetPath, os.ModePerm); err != nil {
				fmt.Fprintln(os.Stderr, "failed to make dir for", targetPath)
			}
		} else {
			targetFile, err := os.Create(filepath.Join(newDir, path))
			if err != nil {
				fmt.Fprintln(os.Stderr, "failed to open file:", path)
				return err
			}
			defer targetFile.Close()

			if strings.Contains(path, filepath.Join(chartPath, "crds")) && info.Name() == "standard.oam.dev_podspecworkloads.yaml" {
				f, err := os.OpenFile(path, os.O_RDONLY, os.ModePerm)
				if err != nil {
					fmt.Fprintln(os.Stderr, "failed to open file", path)
					return err
				}
				defer f.Close()
				r := bufio.NewReader(f)
				var previousLine string
				if previousLine, err = r.ReadString('\n'); err != nil {
					fmt.Fprintln(os.Stderr, "failed to read file line:", err)
					return err
				}
				fmt.Fprint(targetFile, previousLine)

				for {
					line, err := r.ReadString('\n')
					if err != nil {
						if err == io.EOF {
							return nil
						}
						fmt.Fprintln(os.Stderr, "failed to read file line:", err)
						return err
					}

					if strings.Contains(previousLine, "protocol:") &&
						strings.Contains(line, "description: Protocol for port. Must be UDP, TCP,") {
						tmp := strings.Split(line, "description")
						if len(tmp) > 0 {
							blanks := tmp[0]
							defaultStr := blanks + "default: TCP\n"
							fmt.Fprint(targetFile, defaultStr)
						}
					}
					fmt.Fprint(targetFile, line)
					previousLine = line
				}
			} else {
				data, err := ioutil.ReadFile(path)
				if err != nil {
					fmt.Fprintln(os.Stderr, "failed to read file", err)
					return err
				}

				if err = ioutil.WriteFile(targetPath, data, os.ModePerm); err != nil {
					fmt.Fprintln(os.Stderr, "failed to read file:", err)
					return err
				}
			}
		}
		return nil
	})
	if err != nil {
		return "", "", err
	}
	return filepath.Join(newDir, "charts", "vela-core"), newDir, nil
}
