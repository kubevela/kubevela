package main // #nosec

// generate compatibility testdata
import (
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strings"
)

func main() {
	var srcdir, dstdir string
	if len(os.Args) > 1 {
		srcdir = os.Args[1]
		dstdir = os.Args[2]
	}
	err := filepath.Walk(srcdir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			fmt.Println(err)
		}
		if info.IsDir() {
			return nil
		}
		/* #nosec */
		data, err := ioutil.ReadFile(path)
		if err != nil {
			fmt.Fprintln(os.Stderr, "failed to read file", err)
			return err
		}
		fileName := info.Name()
		var newdata string
		if fileName == "core.oam.dev_workloaddefinitions.yaml" || fileName == "core.oam.dev_traitdefinitions.yaml" || fileName == "core.oam.dev_scopedefinitions.yaml" {
			newdata = strings.ReplaceAll(string(data), "scope: Namespaced", "scope: Cluster")
		} else {
			newdata = string(data)
		}
		dstpath := dstdir + "/" + fileName
		/* #nosec */
		if err = ioutil.WriteFile(dstpath, []byte(newdata), 0644); err != nil {
			fmt.Fprintln(os.Stderr, "failed to write file:", err)
			return err
		}
		return nil
	})
	if err != nil {
		log.Fatal(err)
	}
}
