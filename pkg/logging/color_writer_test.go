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

package logging_test

import (
	"bytes"
	"fmt"
	"strings"
	"sync"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/oam-dev/kubevela/pkg/logging"
)

func TestColorWriter(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Color Writer Suite")
}

var _ = Describe("Color Writer", func() {
	Context("formatting", func() {
		It("should format INFO messages with colors", func() {
			var buf bytes.Buffer
			writer := logging.NewColorWriter(&buf)

			input := "I1117 12:34:56.789012    1234 server.go:123] Starting server"
			_, err := writer.Write([]byte(input + "\n"))
			Expect(err).Should(Succeed())

			output := buf.String()
			Expect(output).Should(ContainSubstring("\x1b[32m")) // Info color
			Expect(output).Should(ContainSubstring("INFO"))
			Expect(output).Should(ContainSubstring("Starting server"))
			Expect(output).Should(ContainSubstring("\x1b[0m")) // Reset color
		})

		It("should format ERROR messages with colors", func() {
			var buf bytes.Buffer
			writer := logging.NewColorWriter(&buf)

			input := "E1117 12:34:56.789012    1234 server.go:456] Failed to start server"
			_, err := writer.Write([]byte(input + "\n"))
			Expect(err).Should(Succeed())

			output := buf.String()
			Expect(output).Should(ContainSubstring("\x1b[31m")) // Error color
			Expect(output).Should(ContainSubstring("ERROR"))
			Expect(output).Should(ContainSubstring("Failed to start server"))
		})

		It("should format WARNING messages with colors", func() {
			var buf bytes.Buffer
			writer := logging.NewColorWriter(&buf)

			input := "W1117 12:34:56.789012    1234 server.go:789] Configuration deprecated"
			_, err := writer.Write([]byte(input + "\n"))
			Expect(err).Should(Succeed())

			output := buf.String()
			Expect(output).Should(ContainSubstring("\x1b[33m")) // Warning color
			Expect(output).Should(ContainSubstring("WARN"))
			Expect(output).Should(ContainSubstring("Configuration deprecated"))
		})

		It("should handle structured logging with key-value pairs", func() {
			var buf bytes.Buffer
			writer := logging.NewColorWriter(&buf)

			input := "I1117 12:34:56.789012    1234 hook.go:123] Running hook name=CRDValidation status=success"
			_, err := writer.Write([]byte(input + "\n"))
			Expect(err).Should(Succeed())

			output := buf.String()
			Expect(output).Should(ContainSubstring("Running hook"))
			Expect(output).Should(ContainSubstring("\x1b[96m")) // Key color
			Expect(output).Should(ContainSubstring("name"))
			Expect(output).Should(ContainSubstring("\x1b[37m")) // Value color
			Expect(output).Should(ContainSubstring("CRDValidation"))
		})

		It("should handle quoted values in key-value pairs", func() {
			var buf bytes.Buffer
			writer := logging.NewColorWriter(&buf)

			input := `I1117 12:34:56.789012    1234 server.go:123] Processing request path="/api/v1" method="GET"`
			_, err := writer.Write([]byte(input + "\n"))
			Expect(err).Should(Succeed())

			output := buf.String()
			Expect(output).Should(ContainSubstring("Processing request"))
			Expect(output).Should(ContainSubstring("path"))
			Expect(output).Should(ContainSubstring(`"/api/v1"`))
			Expect(output).Should(ContainSubstring("method"))
			Expect(output).Should(ContainSubstring(`"GET"`))
		})
	})

	Context("concurrency safety", func() {
		It("should handle concurrent writes safely", func() {
			var buf bytes.Buffer
			writer := logging.NewColorWriter(&buf)

			const numGoroutines = 100
			const numWrites = 50

			var wg sync.WaitGroup
			wg.Add(numGoroutines)

			// Use a channel to detect race conditions
			errors := make(chan error, numGoroutines*numWrites)

			for i := 0; i < numGoroutines; i++ {
				go func(id int) {
					defer wg.Done()
					for j := 0; j < numWrites; j++ {
						msg := fmt.Sprintf("I1117 12:34:56.789012    %04d test.go:123] Goroutine %d write %d\n",
							id, id, j)
						_, err := writer.Write([]byte(msg))
						if err != nil {
							errors <- err
						}
					}
				}(i)
			}

			wg.Wait()
			close(errors)

			// Check for any errors
			for err := range errors {
				Expect(err).Should(BeNil())
			}

			// Verify output contains expected number of lines
			output := buf.String()
			lines := bytes.Count([]byte(output), []byte("\n"))
			Expect(lines).Should(Equal(numGoroutines * numWrites))
		})

		It("should maintain data integrity under concurrent access", func() {
			var buf bytes.Buffer
			writer := logging.NewColorWriter(&buf)

			const numGoroutines = 50
			messages := make(map[string]bool)
			var mu sync.Mutex

			var wg sync.WaitGroup
			wg.Add(numGoroutines)

			for i := 0; i < numGoroutines; i++ {
				go func(id int) {
					defer wg.Done()
					// Create unique messages
					for j := 0; j < 10; j++ {
						msg := fmt.Sprintf("I1117 12:34:56.789012    %04d test.go:%03d] Unique message %d-%d",
							id, j*10, id, j)
						mu.Lock()
						messages[msg] = true
						mu.Unlock()

						_, err := writer.Write([]byte(msg + "\n"))
						Expect(err).Should(BeNil())
					}
				}(i)
			}

			wg.Wait()

			// Verify all unique messages appear in output
			output := buf.String()
			for msg := range messages {
				// Extract the unique part of each message (e.g., "Unique message 1-2")
				// The message format is: "I1117 12:34:56.789012    XXXX test.go:YYY] Unique message X-Y"
				// We look for the complete "Unique message X-Y" part
				startIdx := strings.Index(msg, "Unique message")
				if startIdx >= 0 {
					uniquePart := msg[startIdx:] // Get "Unique message X-Y"
					Expect(output).Should(ContainSubstring(uniquePart),
						"Missing message: %s", uniquePart)
				}
			}
		})

		It("should handle partial writes correctly", func() {
			var buf bytes.Buffer
			writer := logging.NewColorWriter(&buf)

			var wg sync.WaitGroup
			wg.Add(2)

			// Goroutine 1: Write a message in parts
			go func() {
				defer wg.Done()
				part1 := []byte("I1117 12:34:56.789012    1234 test.go:123] ")
				part2 := []byte("First ")
				part3 := []byte("message\n")

				writer.Write(part1)
				writer.Write(part2)
				writer.Write(part3)
			}()

			// Goroutine 2: Write a complete message
			go func() {
				defer wg.Done()
				msg := []byte("I1117 12:34:56.789012    5678 test.go:456] Second message\n")
				writer.Write(msg)
			}()

			wg.Wait()

			// Both messages should be in the output
			output := buf.String()
			Expect(output).Should(ContainSubstring("First message"))
			Expect(output).Should(ContainSubstring("Second message"))
		})
	})

	Context("edge cases", func() {
		It("should handle empty input", func() {
			var buf bytes.Buffer
			writer := logging.NewColorWriter(&buf)

			n, err := writer.Write([]byte(""))
			Expect(err).Should(BeNil())
			Expect(n).Should(Equal(0))
			Expect(buf.String()).Should(Equal(""))
		})

		It("should handle input without newlines", func() {
			var buf bytes.Buffer
			writer := logging.NewColorWriter(&buf)

			input := "I1117 12:34:56.789012    1234 test.go:123] Message without newline"
			n, err := writer.Write([]byte(input))
			Expect(err).Should(BeNil())
			Expect(n).Should(Equal(len(input)))

			// The message should be buffered, not yet written to output
			Expect(buf.String()).Should(Equal(""))

			// Write a newline to flush
			writer.Write([]byte("\n"))
			Expect(buf.String()).Should(ContainSubstring("Message without newline"))
		})

		It("should handle malformed log lines gracefully", func() {
			var buf bytes.Buffer
			writer := logging.NewColorWriter(&buf)

			// Various malformed inputs
			inputs := []string{
				"Not a klog line\n",
				"I\n",
				"I1117\n",
				"Random text with no structure\n",
				"\n",
			}

			for _, input := range inputs {
				_, err := writer.Write([]byte(input))
				Expect(err).Should(BeNil())
			}

			// Should have written something for each input
			output := buf.String()
			lines := bytes.Count([]byte(output), []byte("\n"))
			Expect(lines).Should(Equal(len(inputs)))
		})

		It("should handle very long lines", func() {
			var buf bytes.Buffer
			writer := logging.NewColorWriter(&buf)

			// Create a very long message
			longMsg := "I1117 12:34:56.789012    1234 test.go:123] " + string(make([]byte, 10000))
			_, err := writer.Write([]byte(longMsg + "\n"))
			Expect(err).Should(BeNil())

			output := buf.String()
			Expect(len(output)).Should(BeNumerically(">", 10000))
		})
	})
})
