package main

import (
	"bytes"
	"encoding/base64"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"time"

	"github.com/mholt/archiver/v3"
)

func main() {
	tgz := archiver.NewTarGz()
	defer tgz.Close()
	var archiveDir, output string
	flag.StringVar(&archiveDir, "path", "dashboard/dist", "specify frontend static file")
	flag.StringVar(&output, "output", "", "specify output dir, if not set, output base64 result of the gzip result")
	flag.Parse()
	var stdout bool
	if output == "" {
		stdout = true
		output = fmt.Sprintf("vela-frontend-%d.tgz", time.Now().Nanosecond())
	}
	err := tgz.Archive([]string{archiveDir}, output)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	if stdout {
		data, err := ioutil.ReadFile(output)
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
		PrintToFile(base64.StdEncoding.EncodeToString(data))
		_ = os.Remove(output)
	}
}

func PrintToFile(data string) {
	var buffer bytes.Buffer
	buffer.WriteString(`package fake
var FrontendSource = "`)
	FprintZipData(&buffer, []byte(data))
	buffer.WriteString(`"`)
	_ = ioutil.WriteFile("cmd/vela/fake/source.go", buffer.Bytes(), 0644)
}

// From https://github.com/rakyll/statik/blob/master/statik.go#L313
// FprintZipData converts zip binary contents to a string literal.
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
