/*
 Copyright 2023 The KubeVela Authors.

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

package docgen

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"
	"sync"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	"golang.org/x/sync/errgroup"
)

// GenerateProvidersMarkdown generates markdown documentation for providers.
func GenerateProvidersMarkdown(ctx context.Context, providers []io.Reader, w io.Writer) error {
	docs := make([]string, len(providers))
	mu := &sync.Mutex{}
	wg, _ := errgroup.WithContext(ctx)

	for i, provider := range providers {
		i, provider := i, provider
		wg.Go(func() error {
			doc := bytes.NewBuffer(nil)

			if err := GenerateProviderMarkdown(provider, doc); err != nil {
				return err
			}

			mu.Lock()
			docs[i] = doc.String() // stable order
			mu.Unlock()

			return nil
		})
	}

	if err := wg.Wait(); err != nil {
		return err
	}

	_, err := w.Write([]byte(strings.Join(docs, "\n")))
	return err
}

// GenerateProviderMarkdown generates markdown documentation for a provider.
func GenerateProviderMarkdown(provider io.Reader, w io.Writer) error {
	const (
		providerKey = "#provider"
		paramsKey   = "$params"
		returnsKey  = "$returns"
	)

	c := cuecontext.New()

	content, err := io.ReadAll(provider)
	if err != nil {
		return fmt.Errorf("failed to read provider file: %w", err)
	}

	v := c.CompileBytes(content)
	if v.Err() != nil {
		return fmt.Errorf("failed to compile provider file: %w", v.Err())
	}

	// iter provider methods
	iter, err := v.Fields(cue.Definitions(true))
	if err != nil {
		return fmt.Errorf("failed to get definition iterator: %w", err)
	}

	docs, ref, pkg := bytes.NewBuffer(nil), MarkdownReference{}, ""
	for iter.Next() {
		item := iter.Value()

		// get package name. TODO(iyear): more elegant
		if pkg == "" {
			t, err := item.LookupPath(cue.ParsePath(providerKey)).String()
			if err != nil {
				return err
			}
			pkg = t
		}

		// header
		fmt.Fprintf(docs, "## %s\n", iter.Label())

		doc, _, err := ref.parseParameters("", item.LookupPath(cue.ParsePath(paramsKey)), "*Params*", 0, true)
		if err != nil {
			return err
		}
		docs.WriteString(doc)

		doc, _, err = ref.parseParameters("", item.LookupPath(cue.ParsePath(returnsKey)), "*Returns*", 0, true)
		if err != nil {
			return err
		}
		docs.WriteString(doc)
	}

	doc := bytes.NewBuffer(nil)
	fmt.Fprintf(doc, "# %s\n\n", pkg) // package name header
	doc.Write(docs.Bytes())
	doc.WriteString("------\n\n") // footer

	_, err = w.Write(doc.Bytes())
	return err
}
