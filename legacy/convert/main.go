package main // #nosec

import (
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strings"
)

func main() {
	dir, err := os.Getwd()
	if err != nil {
		log.Fatal(err)
	}
	if len(os.Args) > 1 {
		dir = os.Args[1]
	}
	err = filepath.Walk(dir, func(path string, info os.FileInfo, _ error) error {
		if info.IsDir() {
			return nil
		}
		/* #nosec */
		data, err := ioutil.ReadFile(path)
		if err != nil {
			fmt.Fprintln(os.Stderr, "failed to read file", err)
			return err
		}
		newdata := strings.ReplaceAll(string(data), "x-kubernetes-embedded-resource: true", "")
		newdata = strings.ReplaceAll(newdata, "x-kubernetes-preserve-unknown-fields: true", "")
		newdata = strings.ReplaceAll(newdata, "default: false", "")
		/* #nosec */
		if err = ioutil.WriteFile(path, []byte(newdata), 0644); err != nil {
			fmt.Fprintln(os.Stderr, "failed to write file:", err)
			return err
		}
		return nil
	})
	if err != nil {
		log.Fatal(err)
	}
}
