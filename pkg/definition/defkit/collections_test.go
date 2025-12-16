/*
Copyright 2025 The KubeVela Authors.

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

package defkit_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/oam-dev/kubevela/pkg/definition/defkit"
)

var _ = Describe("Collections", func() {

	Describe("Each", func() {
		It("should create a collection operation from a list parameter", func() {
			ports := defkit.List("ports")
			col := defkit.Each(ports)
			Expect(col).NotTo(BeNil())
			Expect(col.Source()).To(Equal(ports))
		})

		It("should chain Filter operation", func() {
			ports := defkit.List("ports")
			col := defkit.Each(ports).Filter(defkit.FieldEquals("expose", true))
			Expect(col.Operations()).To(HaveLen(1))
		})

		It("should chain Map operation", func() {
			ports := defkit.List("ports")
			col := defkit.Each(ports).Map(defkit.FieldMap{
				"containerPort": defkit.FieldRef("port"),
			})
			Expect(col.Operations()).To(HaveLen(1))
		})

		It("should chain multiple operations", func() {
			ports := defkit.List("ports")
			col := defkit.Each(ports).
				Filter(defkit.FieldEquals("expose", true)).
				Map(defkit.FieldMap{
					"port":       defkit.FieldRef("port"),
					"targetPort": defkit.FieldRef("port"),
				}).
				Pick("port", "targetPort")
			Expect(col.Operations()).To(HaveLen(3))
		})

		It("should chain Wrap operation", func() {
			secrets := defkit.StringList("imagePullSecrets")
			col := defkit.Each(secrets).Wrap("name")
			Expect(col.Operations()).To(HaveLen(1))
		})

		It("should chain Rename operation", func() {
			ports := defkit.List("ports")
			col := defkit.Each(ports).Rename("port", "containerPort")
			Expect(col.Operations()).To(HaveLen(1))
		})
	})

	Describe("FieldRef", func() {
		It("should resolve field from item", func() {
			ref := defkit.FieldRef("port")
			Expect(ref).NotTo(BeNil())
		})

		It("should support Or fallback", func() {
			ref := defkit.FieldRef("name").Or(defkit.Format("port-%v", defkit.FieldRef("port")))
			Expect(ref).NotTo(BeNil())
		})
	})

	Describe("FieldEquals", func() {
		It("should create equality predicate", func() {
			pred := defkit.FieldEquals("expose", true)
			Expect(pred).NotTo(BeNil())
		})
	})

	Describe("FieldExists", func() {
		It("should create existence predicate", func() {
			pred := defkit.FieldExists("items")
			Expect(pred).NotTo(BeNil())
		})
	})

	Describe("Format", func() {
		It("should create format field value", func() {
			f := defkit.Format("port-%v", defkit.FieldRef("port"))
			Expect(f).NotTo(BeNil())
		})
	})

	Describe("LitField", func() {
		It("should create literal field value", func() {
			lit := defkit.LitField("TCP")
			Expect(lit).NotTo(BeNil())
		})
	})

	Describe("FromFields", func() {
		It("should create multi-source collection", func() {
			volumeMounts := defkit.Object("volumeMounts")
			ms := defkit.FromFields(volumeMounts, "pvc", "configMap", "secret")
			Expect(ms).NotTo(BeNil())
			Expect(ms.Source()).To(Equal(volumeMounts))
			Expect(ms.Sources()).To(Equal([]string{"pvc", "configMap", "secret"}))
		})

		It("should chain Pick operation", func() {
			volumeMounts := defkit.Object("volumeMounts")
			ms := defkit.FromFields(volumeMounts, "pvc", "configMap").
				Pick("name", "mountPath")
			Expect(ms.Operations()).To(HaveLen(1))
		})

		It("should chain Dedupe operation", func() {
			volumeMounts := defkit.Object("volumeMounts")
			ms := defkit.FromFields(volumeMounts, "pvc", "configMap").
				Dedupe("name")
			Expect(ms.Operations()).To(HaveLen(1))
		})

		It("should chain MapBySource operation", func() {
			volumeMounts := defkit.Object("volumeMounts")
			ms := defkit.FromFields(volumeMounts, "pvc", "configMap").
				MapBySource(map[string]defkit.FieldMap{
					"pvc": {
						"name":                  defkit.FieldRef("name"),
						"persistentVolumeClaim": defkit.Nested(defkit.FieldMap{"claimName": defkit.FieldRef("claimName")}),
					},
					"configMap": {
						"name":      defkit.FieldRef("name"),
						"configMap": defkit.Nested(defkit.FieldMap{"name": defkit.FieldRef("cmName")}),
					},
				})
			Expect(ms.MapBySourceMappings()).To(HaveLen(2))
		})
	})

	Describe("Nested", func() {
		It("should create nested field mapping", func() {
			nested := defkit.Nested(defkit.FieldMap{
				"claimName": defkit.FieldRef("claimName"),
			})
			Expect(nested).NotTo(BeNil())
		})
	})

	Describe("Optional", func() {
		It("should create optional field reference", func() {
			opt := defkit.Optional("items")
			Expect(opt).NotTo(BeNil())
		})
	})

	Describe("Collection Resolution", func() {
		var (
			ports    *defkit.ArrayParam
			comp     *defkit.ComponentDefinition
		)

		BeforeEach(func() {
			ports = defkit.List("ports").WithFields(
				defkit.Int("port"),
				defkit.String("name"),
				defkit.String("protocol").Default("TCP"),
				defkit.Bool("expose").Default(false),
			)
		})

		It("should resolve Each().Map() transformation", func() {
			comp = defkit.NewComponent("test").
				Params(ports).
				Template(func(tpl *defkit.Template) {
					containerPorts := defkit.Each(ports).Map(defkit.FieldMap{
						"containerPort": defkit.FieldRef("port"),
						"name":          defkit.FieldRef("name"),
						"protocol":      defkit.FieldRef("protocol"),
					})
					tpl.Output(
						defkit.NewResource("apps/v1", "Deployment").
							Set("spec.template.spec.containers[0].ports", containerPorts),
					)
				})

			rendered := comp.Render(
				defkit.TestContext().
					WithParam("ports", []map[string]any{
						{"port": 80, "name": "http", "protocol": "TCP", "expose": true},
						{"port": 443, "name": "https", "protocol": "TCP", "expose": false},
					}),
			)

			containerPorts := rendered.Get("spec.template.spec.containers[0].ports").([]any)
			Expect(containerPorts).To(HaveLen(2))
			Expect(containerPorts[0].(map[string]any)["containerPort"]).To(Equal(80))
			Expect(containerPorts[0].(map[string]any)["name"]).To(Equal("http"))
			Expect(containerPorts[1].(map[string]any)["containerPort"]).To(Equal(443))
		})

		It("should resolve Each().Filter().Map() transformation", func() {
			comp = defkit.NewComponent("test").
				Params(ports).
				Template(func(tpl *defkit.Template) {
					servicePorts := defkit.Each(ports).
						Filter(defkit.FieldEquals("expose", true)).
						Map(defkit.FieldMap{
							"port":       defkit.FieldRef("port"),
							"targetPort": defkit.FieldRef("port"),
							"name":       defkit.FieldRef("name"),
						})
					tpl.Output(
						defkit.NewResource("v1", "Service").
							Set("spec.ports", servicePorts),
					)
				})

			rendered := comp.Render(
				defkit.TestContext().
					WithParam("ports", []map[string]any{
						{"port": 80, "name": "http", "protocol": "TCP", "expose": true},
						{"port": 443, "name": "https", "protocol": "TCP", "expose": false},
						{"port": 8080, "name": "admin", "protocol": "TCP", "expose": true},
					}),
			)

			servicePorts := rendered.Get("spec.ports").([]any)
			Expect(servicePorts).To(HaveLen(2)) // Only expose=true ports
			Expect(servicePorts[0].(map[string]any)["port"]).To(Equal(80))
			Expect(servicePorts[1].(map[string]any)["port"]).To(Equal(8080))
		})

		It("should resolve Each().Wrap() transformation", func() {
			secrets := defkit.StringList("imagePullSecrets")
			comp = defkit.NewComponent("test").
				Params(secrets).
				Template(func(tpl *defkit.Template) {
					pullSecrets := defkit.Each(secrets).Wrap("name")
					tpl.Output(
						defkit.NewResource("apps/v1", "Deployment").
							Set("spec.template.spec.imagePullSecrets", pullSecrets),
					)
				})

			rendered := comp.Render(
				defkit.TestContext().
					WithParam("imagePullSecrets", []any{"docker-secret", "gcr-secret"}),
			)

			pullSecrets := rendered.Get("spec.template.spec.imagePullSecrets").([]any)
			Expect(pullSecrets).To(HaveLen(2))
			Expect(pullSecrets[0].(map[string]any)["name"]).To(Equal("docker-secret"))
			Expect(pullSecrets[1].(map[string]any)["name"]).To(Equal("gcr-secret"))
		})
	})

	Describe("MultiSource Resolution", func() {
		var (
			volumeMounts *defkit.MapParam
			comp         *defkit.ComponentDefinition
		)

		BeforeEach(func() {
			volumeMounts = defkit.Object("volumeMounts")
		})

		It("should resolve FromFields().Pick().Dedupe() for container mounts", func() {
			comp = defkit.NewComponent("test").
				Params(volumeMounts).
				Template(func(tpl *defkit.Template) {
					containerMounts := defkit.FromFields(volumeMounts, "pvc", "configMap", "secret").
						Pick("name", "mountPath", "subPath").
						Dedupe("name")
					tpl.Output(
						defkit.NewResource("apps/v1", "Deployment").
							Set("spec.template.spec.containers[0].volumeMounts", containerMounts),
					)
				})

			rendered := comp.Render(
				defkit.TestContext().
					WithParam("volumeMounts", map[string]any{
						"pvc": []map[string]any{
							{"name": "data", "mountPath": "/data", "claimName": "data-pvc"},
						},
						"configMap": []map[string]any{
							{"name": "config", "mountPath": "/etc/config", "cmName": "app-config"},
						},
						"secret": []map[string]any{
							{"name": "creds", "mountPath": "/etc/creds", "secretName": "app-secret"},
						},
					}),
			)

			mounts := rendered.Get("spec.template.spec.containers[0].volumeMounts").([]any)
			Expect(mounts).To(HaveLen(3))
			// Verify Pick only included specified fields
			Expect(mounts[0].(map[string]any)).To(HaveKey("name"))
			Expect(mounts[0].(map[string]any)).To(HaveKey("mountPath"))
			Expect(mounts[0].(map[string]any)).NotTo(HaveKey("claimName"))
		})

		It("should resolve FromFields().MapBySource() for pod volumes", func() {
			comp = defkit.NewComponent("test").
				Params(volumeMounts).
				Template(func(tpl *defkit.Template) {
					podVolumes := defkit.FromFields(volumeMounts, "pvc", "configMap", "emptyDir").
						MapBySource(map[string]defkit.FieldMap{
							"pvc": {
								"name":                  defkit.FieldRef("name"),
								"persistentVolumeClaim": defkit.Nested(defkit.FieldMap{"claimName": defkit.FieldRef("claimName")}),
							},
							"configMap": {
								"name": defkit.FieldRef("name"),
								"configMap": defkit.Nested(defkit.FieldMap{
									"name":        defkit.FieldRef("cmName"),
									"defaultMode": defkit.FieldRef("defaultMode"),
								}),
							},
							"emptyDir": {
								"name":     defkit.FieldRef("name"),
								"emptyDir": defkit.Nested(defkit.FieldMap{"medium": defkit.FieldRef("medium")}),
							},
						}).
						Dedupe("name")
					tpl.Output(
						defkit.NewResource("apps/v1", "Deployment").
							Set("spec.template.spec.volumes", podVolumes),
					)
				})

			rendered := comp.Render(
				defkit.TestContext().
					WithParam("volumeMounts", map[string]any{
						"pvc": []map[string]any{
							{"name": "data", "mountPath": "/data", "claimName": "data-pvc"},
						},
						"configMap": []map[string]any{
							{"name": "config", "mountPath": "/etc/config", "cmName": "app-config", "defaultMode": 420},
						},
						"emptyDir": []map[string]any{
							{"name": "cache", "mountPath": "/cache", "medium": "Memory"},
						},
					}),
			)

			volumes := rendered.Get("spec.template.spec.volumes").([]any)
			Expect(volumes).To(HaveLen(3))

			// Verify PVC volume structure
			pvcVol := volumes[0].(map[string]any)
			Expect(pvcVol["name"]).To(Equal("data"))
			Expect(pvcVol["persistentVolumeClaim"].(map[string]any)["claimName"]).To(Equal("data-pvc"))

			// Verify ConfigMap volume structure
			cmVol := volumes[1].(map[string]any)
			Expect(cmVol["name"]).To(Equal("config"))
			Expect(cmVol["configMap"].(map[string]any)["name"]).To(Equal("app-config"))
			Expect(cmVol["configMap"].(map[string]any)["defaultMode"]).To(Equal(420))

			// Verify EmptyDir volume structure
			emptyVol := volumes[2].(map[string]any)
			Expect(emptyVol["name"]).To(Equal("cache"))
			Expect(emptyVol["emptyDir"].(map[string]any)["medium"]).To(Equal("Memory"))
		})

		It("should dedupe volumes by name", func() {
			comp = defkit.NewComponent("test").
				Params(volumeMounts).
				Template(func(tpl *defkit.Template) {
					podVolumes := defkit.FromFields(volumeMounts, "pvc", "configMap").
						MapBySource(map[string]defkit.FieldMap{
							"pvc": {
								"name":                  defkit.FieldRef("name"),
								"persistentVolumeClaim": defkit.Nested(defkit.FieldMap{"claimName": defkit.FieldRef("claimName")}),
							},
							"configMap": {
								"name":      defkit.FieldRef("name"),
								"configMap": defkit.Nested(defkit.FieldMap{"name": defkit.FieldRef("cmName")}),
							},
						}).
						Dedupe("name")
					tpl.Output(
						defkit.NewResource("apps/v1", "Deployment").
							Set("spec.template.spec.volumes", podVolumes),
					)
				})

			rendered := comp.Render(
				defkit.TestContext().
					WithParam("volumeMounts", map[string]any{
						"pvc": []map[string]any{
							{"name": "shared", "claimName": "pvc-1"},
							{"name": "data", "claimName": "pvc-2"},
						},
						"configMap": []map[string]any{
							{"name": "shared", "cmName": "cm-1"}, // Duplicate name - should be deduped
							{"name": "config", "cmName": "cm-2"},
						},
					}),
			)

			volumes := rendered.Get("spec.template.spec.volumes").([]any)
			Expect(volumes).To(HaveLen(3)) // "shared" appears only once (from pvc, first occurrence wins)

			names := make([]string, len(volumes))
			for i, v := range volumes {
				names[i] = v.(map[string]any)["name"].(string)
			}
			Expect(names).To(ConsistOf("shared", "data", "config"))
		})

		It("should handle Optional fields in Nested mappings", func() {
			comp = defkit.NewComponent("test").
				Params(volumeMounts).
				Template(func(tpl *defkit.Template) {
					podVolumes := defkit.FromFields(volumeMounts, "configMap").
						MapBySource(map[string]defkit.FieldMap{
							"configMap": {
								"name": defkit.FieldRef("name"),
								"configMap": defkit.Nested(defkit.FieldMap{
									"name":        defkit.FieldRef("cmName"),
									"defaultMode": defkit.FieldRef("defaultMode"),
									"items":       defkit.Optional("items"), // Optional field
								}),
							},
						})
					tpl.Output(
						defkit.NewResource("apps/v1", "Deployment").
							Set("spec.template.spec.volumes", podVolumes),
					)
				})

			// Test with items present
			rendered := comp.Render(
				defkit.TestContext().
					WithParam("volumeMounts", map[string]any{
						"configMap": []map[string]any{
							{
								"name":        "with-items",
								"cmName":      "cm-1",
								"defaultMode": 420,
								"items":       []map[string]any{{"key": "app.conf", "path": "app.conf"}},
							},
							{
								"name":        "without-items",
								"cmName":      "cm-2",
								"defaultMode": 420,
								// No items field
							},
						},
					}),
			)

			volumes := rendered.Get("spec.template.spec.volumes").([]any)
			Expect(volumes).To(HaveLen(2))

			// Volume with items should have items field
			volWithItems := volumes[0].(map[string]any)
			Expect(volWithItems["configMap"].(map[string]any)).To(HaveKey("items"))

			// Volume without items should not have items field
			volWithoutItems := volumes[1].(map[string]any)
			Expect(volWithoutItems["configMap"].(map[string]any)).NotTo(HaveKey("items"))
		})
	})

	// --- Go 1.23 Iterator Methods Tests ---
	Describe("Go 1.23 Iterator Methods", func() {
		Describe("CollectionOp.All", func() {
			It("should iterate over all items using iter.Seq", func() {
				ports := defkit.List("ports")
				col := defkit.Each(ports).
					Filter(defkit.FieldEquals("expose", true))

				items := []any{
					map[string]any{"port": 80, "expose": true},
					map[string]any{"port": 443, "expose": false},
					map[string]any{"port": 8080, "expose": true},
				}

				var results []map[string]any
				for item := range col.All(items) {
					results = append(results, item)
				}

				Expect(results).To(HaveLen(2))
				Expect(results[0]["port"]).To(Equal(80))
				Expect(results[1]["port"]).To(Equal(8080))
			})

			It("should support early termination", func() {
				ports := defkit.List("ports")
				col := defkit.Each(ports)

				items := []any{
					map[string]any{"port": 80},
					map[string]any{"port": 443},
					map[string]any{"port": 8080},
				}

				count := 0
				for range col.All(items) {
					count++
					if count == 2 {
						break
					}
				}

				Expect(count).To(Equal(2))
			})
		})

		Describe("CollectionOp.AllPairs", func() {
			It("should iterate with index and item using iter.Seq2", func() {
				ports := defkit.List("ports")
				col := defkit.Each(ports)

				items := []any{
					map[string]any{"port": 80},
					map[string]any{"port": 443},
				}

				var indices []int
				var ports_list []int
				for i, item := range col.AllPairs(items) {
					indices = append(indices, i)
					ports_list = append(ports_list, item["port"].(int))
				}

				Expect(indices).To(Equal([]int{0, 1}))
				Expect(ports_list).To(Equal([]int{80, 443}))
			})
		})

		Describe("CollectionOp.Collect", func() {
			It("should materialize iterator to slice using slices.Collect", func() {
				ports := defkit.List("ports")
				col := defkit.Each(ports).
					Filter(defkit.FieldEquals("expose", true))

				items := []any{
					map[string]any{"port": 80, "expose": true},
					map[string]any{"port": 443, "expose": false},
				}

				results := col.Collect(items)
				Expect(results).To(HaveLen(1))
				Expect(results[0]["port"]).To(Equal(80))
			})
		})

		Describe("CollectionOp.Count", func() {
			It("should count items after applying operations", func() {
				ports := defkit.List("ports")
				col := defkit.Each(ports).
					Filter(defkit.FieldEquals("expose", true))

				items := []any{
					map[string]any{"port": 80, "expose": true},
					map[string]any{"port": 443, "expose": false},
					map[string]any{"port": 8080, "expose": true},
				}

				Expect(col.Count(items)).To(Equal(2))
			})
		})

		Describe("CollectionOp.First", func() {
			It("should return first item after applying operations", func() {
				ports := defkit.List("ports")
				col := defkit.Each(ports).
					Filter(defkit.FieldEquals("expose", true))

				items := []any{
					map[string]any{"port": 80, "expose": true},
					map[string]any{"port": 443, "expose": false},
				}

				first := col.First(items)
				Expect(first).NotTo(BeNil())
				Expect(first["port"]).To(Equal(80))
			})

			It("should return nil for empty result", func() {
				ports := defkit.List("ports")
				col := defkit.Each(ports).
					Filter(defkit.FieldEquals("expose", true))

				items := []any{
					map[string]any{"port": 443, "expose": false},
				}

				first := col.First(items)
				Expect(first).To(BeNil())
			})
		})

		Describe("FieldMap iterators", func() {
			It("should iterate over all key-value pairs using maps.All", func() {
				fm := defkit.FieldMap{
					"containerPort": defkit.FieldRef("port"),
					"name":          defkit.FieldRef("name"),
				}

				var keys []string
				for key := range fm.Keys() {
					keys = append(keys, key)
				}

				Expect(keys).To(HaveLen(2))
				Expect(keys).To(ContainElements("containerPort", "name"))
			})
		})

		Describe("MultiSource.All", func() {
			It("should iterate over all items from multiple sources", func() {
				volumeMounts := defkit.Object("volumeMounts")
				ms := defkit.FromFields(volumeMounts, "pvc", "configMap").
					Pick("name", "mountPath")

				sourceData := map[string]any{
					"pvc": []map[string]any{
						{"name": "data", "mountPath": "/data", "claimName": "data-pvc"},
					},
					"configMap": []map[string]any{
						{"name": "config", "mountPath": "/etc/config", "cmName": "app-config"},
					},
				}

				var results []map[string]any
				for item := range ms.All(sourceData) {
					results = append(results, item)
				}

				Expect(results).To(HaveLen(2))
				// Verify Pick only included specified fields
				for _, r := range results {
					Expect(r).To(HaveKey("name"))
					Expect(r).To(HaveKey("mountPath"))
					Expect(r).NotTo(HaveKey("claimName"))
					Expect(r).NotTo(HaveKey("cmName"))
				}
			})
		})

		Describe("MultiSource.Collect", func() {
			It("should materialize iterator to slice", func() {
				volumeMounts := defkit.Object("volumeMounts")
				ms := defkit.FromFields(volumeMounts, "pvc", "configMap")

				sourceData := map[string]any{
					"pvc": []map[string]any{
						{"name": "data"},
					},
					"configMap": []map[string]any{
						{"name": "config"},
					},
				}

				results := ms.Collect(sourceData)
				Expect(results).To(HaveLen(2))
			})
		})

		Describe("MultiSource.Count", func() {
			It("should count items from all sources", func() {
				volumeMounts := defkit.Object("volumeMounts")
				ms := defkit.FromFields(volumeMounts, "pvc", "configMap", "secret")

				sourceData := map[string]any{
					"pvc": []map[string]any{
						{"name": "data1"},
						{"name": "data2"},
					},
					"configMap": []map[string]any{
						{"name": "config"},
					},
					// secret is empty
				}

				Expect(ms.Count(sourceData)).To(Equal(3))
			})
		})
	})
})
