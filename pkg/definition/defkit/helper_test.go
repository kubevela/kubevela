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

var _ = Describe("Helper", func() {

	Describe("HelperVar Attributes", func() {
		var tpl *defkit.Template

		BeforeEach(func() {
			tpl = defkit.NewTemplate()
		})

		It("should have all attributes correctly set for a basic helper", func() {
			ports := defkit.List("ports")
			helper := tpl.Helper("portsArray").
				From(ports).
				Build()

			// Test ALL attributes of HelperVar
			Expect(helper.Name()).To(Equal("portsArray"))
			Expect(helper.IsAfterOutput()).To(BeFalse())
			Expect(helper.Guard()).To(BeNil())
			Expect(helper.String()).To(Equal("Helper(portsArray)"))

			// Verify collection is set and correct
			col, ok := helper.Collection().(*defkit.CollectionOp)
			Expect(ok).To(BeTrue(), "Collection should be a CollectionOp")
			Expect(col.Source()).To(Equal(ports))
			Expect(col.Operations()).To(BeEmpty())
		})

		It("should have all attributes correctly set for a fully configured helper", func() {
			ports := defkit.List("ports")
			guardCond := ports.IsSet()
			helper := tpl.Helper("exposePorts").
				From(ports).
				Guard(guardCond).
				FilterPred(defkit.FieldEquals("expose", true)).
				Pick("name", "port").
				AfterOutput().
				Build()

			// Verify ALL attributes
			Expect(helper.Name()).To(Equal("exposePorts"))
			Expect(helper.IsAfterOutput()).To(BeTrue())
			Expect(helper.Guard()).To(Equal(guardCond))
			Expect(helper.String()).To(Equal("Helper(exposePorts)"))

			// Verify collection has correct operations
			col, ok := helper.Collection().(*defkit.CollectionOp)
			Expect(ok).To(BeTrue())
			Expect(col.Source()).To(Equal(ports))
			Expect(col.Operations()).To(HaveLen(2)) // FilterPred + Pick
		})

		It("should create LenNotZeroCondition with helper as source via NotEmpty()", func() {
			ports := defkit.List("ports")
			helper := tpl.Helper("portsArray").From(ports).Build()

			cond := helper.NotEmpty()
			lenCond, ok := cond.(*defkit.LenNotZeroCondition)
			Expect(ok).To(BeTrue())
			Expect(lenCond.Source()).To(Equal(helper))
		})
	})

	Describe("HelperBuilder with From source - Attributes", func() {
		var tpl *defkit.Template

		BeforeEach(func() {
			tpl = defkit.NewTemplate()
		})

		It("should create CollectionOp with correct source", func() {
			ports := defkit.List("ports")
			helper := tpl.Helper("portsArray").From(ports).Build()

			col, ok := helper.Collection().(*defkit.CollectionOp)
			Expect(ok).To(BeTrue())
			Expect(col.Source()).To(Equal(ports))
			Expect(col.Operations()).To(BeEmpty())
		})

		It("should track multiple operations in sequence", func() {
			ports := defkit.List("ports")
			helper := tpl.Helper("portsArray").
				From(ports).
				FilterPred(defkit.FieldEquals("expose", true)).
				Pick("name", "port").
				Rename("port", "containerPort").
				DefaultField("protocol", defkit.LitField("TCP")).
				Wrap("container").
				Dedupe("name").
				Build()

			col := helper.Collection().(*defkit.CollectionOp)
			// Filter + Pick + Rename + DefaultField + Wrap + Dedupe = 6 operations
			Expect(col.Operations()).To(HaveLen(6))
		})
	})

	Describe("HelperBuilder with FromFields source - Attributes", func() {
		var tpl *defkit.Template

		BeforeEach(func() {
			tpl = defkit.NewTemplate()
		})

		It("should create MultiSource with all specified fields", func() {
			volumeMounts := defkit.Object("volumeMounts")
			helper := tpl.Helper("mountsArray").
				FromFields(volumeMounts, "pvc", "configMap", "secret", "emptyDir", "hostPath").
				Build()

			ms, ok := helper.Collection().(*defkit.MultiSource)
			Expect(ok).To(BeTrue())
			Expect(ms.Source()).To(Equal(volumeMounts))
			Expect(ms.Sources()).To(Equal([]string{"pvc", "configMap", "secret", "emptyDir", "hostPath"}))
			Expect(ms.Operations()).To(BeEmpty())
			Expect(ms.MapBySourceMappings()).To(BeEmpty())
		})

		It("should store MapBySource mappings correctly", func() {
			volumeMounts := defkit.Object("volumeMounts")
			pvcMapping := defkit.FieldMap{
				"name":      defkit.FieldRef("name"),
				"mountPath": defkit.FieldRef("mountPath"),
				"claimName": defkit.FieldRef("claimName"),
			}
			cmMapping := defkit.FieldMap{
				"name":      defkit.FieldRef("name"),
				"mountPath": defkit.FieldRef("mountPath"),
				"cmName":    defkit.FieldRef("cmName"),
			}
			helper := tpl.Helper("volumesList").
				FromFields(volumeMounts, "pvc", "configMap").
				MapBySource(map[string]defkit.FieldMap{
					"pvc":       pvcMapping,
					"configMap": cmMapping,
				}).
				Build()

			ms := helper.Collection().(*defkit.MultiSource)
			Expect(ms.MapBySourceMappings()).To(HaveLen(2))
			Expect(ms.MapBySourceMappings()["pvc"]).To(Equal(pvcMapping))
			Expect(ms.MapBySourceMappings()["configMap"]).To(Equal(cmMapping))
		})
	})

	Describe("HelperBuilder with FromHelper source - Attributes", func() {
		var tpl *defkit.Template

		BeforeEach(func() {
			tpl = defkit.NewTemplate()
		})

		It("should create CollectionOp referencing the source helper", func() {
			ports := defkit.List("ports")
			sourceHelper := tpl.Helper("portsArray").From(ports).Build()

			derivedHelper := tpl.Helper("dedupedPorts").
				FromHelper(sourceHelper).
				Dedupe("name").
				Build()

			col, ok := derivedHelper.Collection().(*defkit.CollectionOp)
			Expect(ok).To(BeTrue())
			Expect(col.Source()).To(Equal(sourceHelper))
			Expect(col.Operations()).To(HaveLen(1))
		})
	})

	Describe("HelperBuilder without source - Attributes", func() {
		It("should create CollectionOp with empty literal array", func() {
			tpl := defkit.NewTemplate()
			helper := tpl.Helper("emptyHelper").Build()

			col, ok := helper.Collection().(*defkit.CollectionOp)
			Expect(ok).To(BeTrue())
			lit, ok := col.Source().(*defkit.Literal)
			Expect(ok).To(BeTrue())
			Expect(lit.Val()).To(Equal([]any{}))
		})
	})

	Describe("Template Helper Registration - Behavior", func() {
		var tpl *defkit.Template

		BeforeEach(func() {
			tpl = defkit.NewTemplate()
		})

		It("should register helpers in order and make them all retrievable", func() {
			ports := defkit.List("ports")
			helper1 := tpl.Helper("helper1").From(ports).Build()
			helper2 := tpl.Helper("helper2").From(ports).Build()
			helper3 := tpl.Helper("helper3").From(ports).Build()

			helpers := tpl.GetHelpers()
			Expect(helpers).To(HaveLen(3))
			Expect(helpers[0]).To(Equal(helper1))
			Expect(helpers[1]).To(Equal(helper2))
			Expect(helpers[2]).To(Equal(helper3))
		})

		It("should correctly categorize helpers by placement", func() {
			ports := defkit.List("ports")
			before1 := tpl.Helper("before1").From(ports).Build()
			after1 := tpl.Helper("after1").From(ports).AfterOutput().Build()
			before2 := tpl.Helper("before2").From(ports).Build()
			after2 := tpl.Helper("after2").From(ports).AfterOutput().Build()

			beforeHelpers := tpl.GetHelpersBeforeOutput()
			Expect(beforeHelpers).To(HaveLen(2))
			Expect(beforeHelpers[0]).To(Equal(before1))
			Expect(beforeHelpers[1]).To(Equal(before2))

			afterHelpers := tpl.GetHelpersAfterOutput()
			Expect(afterHelpers).To(HaveLen(2))
			Expect(afterHelpers[0]).To(Equal(after1))
			Expect(afterHelpers[1]).To(Equal(after2))
		})
	})

	Describe("StructBuilder and StructFieldDef - Attributes", func() {
		It("should create struct with all field attributes preserved", func() {
			nameVal := defkit.Item().Get("name")
			pathVal := defkit.Item().Get("mountPath")
			subPathVal := defkit.Item().Get("subPath")
			subPathCond := defkit.ItemFieldIsSet("subPath")

			s := defkit.HelperStruct(
				defkit.HelperField("name", nameVal),
				defkit.HelperField("mountPath", pathVal),
				defkit.HelperFieldIf(subPathCond, "subPath", subPathVal),
			)

			fields := s.Fields()
			Expect(fields).To(HaveLen(3))

			// First field - all attributes
			Expect(fields[0].Name()).To(Equal("name"))
			Expect(fields[0].Value()).To(Equal(nameVal))
			Expect(fields[0].Cond()).To(BeNil())

			// Second field - all attributes
			Expect(fields[1].Name()).To(Equal("mountPath"))
			Expect(fields[1].Value()).To(Equal(pathVal))
			Expect(fields[1].Cond()).To(BeNil())

			// Third field - all attributes including condition
			Expect(fields[2].Name()).To(Equal("subPath"))
			Expect(fields[2].Value()).To(Equal(subPathVal))
			Expect(fields[2].Cond()).To(Equal(subPathCond))
		})
	})

	Describe("ItemValue - Attributes", func() {
		It("should create item reference with empty field", func() {
			item := defkit.Item()
			Expect(item.Field()).To(Equal(""))
		})

		It("should create field reference with correct field name", func() {
			item := defkit.Item()
			fieldRef := item.Get("name")
			Expect(fieldRef.Field()).To(Equal("name"))
		})
	})

	Describe("ItemFieldIsSet - Attributes", func() {
		It("should create IsSetCondition with correct field name", func() {
			cond := defkit.ItemFieldIsSet("protocol")
			isSet, ok := cond.(*defkit.IsSetCondition)
			Expect(ok).To(BeTrue())
			Expect(isSet.ParamName()).To(Equal("protocol"))
		})
	})

	Describe("StructArrayHelper - Attributes", func() {
		var tpl *defkit.Template

		BeforeEach(func() {
			tpl = defkit.NewTemplate()
		})

		It("should create helper with all attributes correctly set", func() {
			volumeMounts := defkit.Object("volumeMounts")
			pvcMapping := defkit.FieldMap{
				"name":      defkit.FieldRef("name"),
				"mountPath": defkit.FieldRef("mountPath"),
			}
			cmMapping := defkit.FieldMap{
				"name":      defkit.FieldRef("name"),
				"mountPath": defkit.FieldRef("mountPath"),
				"cmName":    defkit.FieldRef("cmName"),
			}

			helper := tpl.StructArrayHelper("mountsArray", volumeMounts).
				Field("pvc", pvcMapping).
				Field("configMap", cmMapping).
				Build()

			// Verify ALL attributes
			Expect(helper.HelperName()).To(Equal("mountsArray"))
			Expect(helper.Source()).To(Equal(volumeMounts))

			fields := helper.Fields()
			Expect(fields).To(HaveLen(2))

			// First field - all attributes
			Expect(fields[0].Name).To(Equal("pvc"))
			Expect(fields[0].Mappings).To(HaveLen(2))
			Expect(fields[0].Mappings["name"]).To(Equal(defkit.FieldRef("name")))
			Expect(fields[0].Mappings["mountPath"]).To(Equal(defkit.FieldRef("mountPath")))

			// Second field - all attributes
			Expect(fields[1].Name).To(Equal("configMap"))
			Expect(fields[1].Mappings).To(HaveLen(3))
		})

		It("should register with template", func() {
			volumeMounts := defkit.Object("volumeMounts")
			helper := tpl.StructArrayHelper("mountsArray", volumeMounts).
				Field("pvc", defkit.FieldMap{"name": defkit.FieldRef("name")}).
				Build()

			registered := tpl.GetStructArrayHelpers()
			Expect(registered).To(HaveLen(1))
			Expect(registered[0]).To(Equal(helper))
		})
	})

	Describe("ConcatHelper - Attributes", func() {
		var tpl *defkit.Template
		var structHelper *defkit.StructArrayHelper

		BeforeEach(func() {
			tpl = defkit.NewTemplate()
			volumeMounts := defkit.Object("volumeMounts")
			structHelper = tpl.StructArrayHelper("mountsArray", volumeMounts).
				Field("pvc", defkit.FieldMap{"name": defkit.FieldRef("name")}).
				Field("configMap", defkit.FieldMap{"name": defkit.FieldRef("name")}).
				Field("secret", defkit.FieldMap{"name": defkit.FieldRef("name")}).
				Build()
		})

		It("should create helper with all attributes correctly set", func() {
			concatHelper := tpl.ConcatHelper("volumesList", structHelper).
				Fields("pvc", "configMap", "secret").
				Build()

			// Verify ALL attributes
			Expect(concatHelper.HelperName()).To(Equal("volumesList"))
			Expect(concatHelper.Source()).To(Equal(structHelper))
			Expect(concatHelper.FieldRefs()).To(Equal([]string{"pvc", "configMap", "secret"}))
			Expect(concatHelper.RequiredImports()).To(Equal([]string{"list"}))
		})

		It("should register with template", func() {
			helper := tpl.ConcatHelper("volumesList", structHelper).
				Fields("pvc").
				Build()

			registered := tpl.GetConcatHelpers()
			Expect(registered).To(HaveLen(1))
			Expect(registered[0]).To(Equal(helper))
		})
	})

	Describe("DedupeHelper - Attributes", func() {
		var tpl *defkit.Template
		var concatHelper *defkit.ConcatHelper

		BeforeEach(func() {
			tpl = defkit.NewTemplate()
			volumeMounts := defkit.Object("volumeMounts")
			structHelper := tpl.StructArrayHelper("mountsArray", volumeMounts).
				Field("pvc", defkit.FieldMap{"name": defkit.FieldRef("name")}).
				Build()
			concatHelper = tpl.ConcatHelper("volumesList", structHelper).
				Fields("pvc").
				Build()
		})

		It("should create helper with all attributes correctly set", func() {
			dedupeHelper := tpl.DedupeHelper("deDupVolumes", concatHelper).
				ByKey("name").
				Build()

			// Verify ALL attributes
			Expect(dedupeHelper.HelperName()).To(Equal("deDupVolumes"))
			Expect(dedupeHelper.Source()).To(Equal(concatHelper))
			Expect(dedupeHelper.KeyField()).To(Equal("name"))
		})

		It("should register with template", func() {
			helper := tpl.DedupeHelper("deDupVolumes", concatHelper).
				ByKey("name").
				Build()

			registered := tpl.GetDedupeHelpers()
			Expect(registered).To(HaveLen(1))
			Expect(registered[0]).To(Equal(helper))
		})

		It("should accept HelperVar as source", func() {
			ports := defkit.List("ports")
			portsHelper := tpl.Helper("portsArray").From(ports).Build()

			dedupeHelper := tpl.DedupeHelper("uniquePorts", portsHelper).
				ByKey("id").
				Build()

			Expect(dedupeHelper.HelperName()).To(Equal("uniquePorts"))
			Expect(dedupeHelper.Source()).To(Equal(portsHelper))
			Expect(dedupeHelper.KeyField()).To(Equal("id"))
		})
	})

	// Behavioral Tests - Test actual data transformations
	Describe("Helper Operations - Behavioral Tests", func() {
		Describe("Filter behavior via component render", func() {
			It("should filter items based on predicate when rendered", func() {
				ports := defkit.List("ports").WithFields(
					defkit.Int("port"),
					defkit.String("name"),
					defkit.Bool("expose").Default(false),
				)

				comp := defkit.NewComponent("test").
					Params(ports).
					Template(func(tpl *defkit.Template) {
						exposedPorts := defkit.Each(ports).
							Filter(defkit.FieldEquals("expose", true)).
							Map(defkit.FieldMap{
								"port": defkit.FieldRef("port"),
								"name": defkit.FieldRef("name"),
							})
						tpl.Output(
							defkit.NewResource("v1", "Service").
								Set("spec.ports", exposedPorts),
						)
					})

				rendered := comp.Render(
					defkit.TestContext().
						WithParam("ports", []map[string]any{
							{"port": 80, "name": "http", "expose": true},
							{"port": 443, "name": "https", "expose": false},
							{"port": 8080, "name": "admin", "expose": true},
						}),
				)

				servicePorts := rendered.Get("spec.ports").([]any)
				Expect(servicePorts).To(HaveLen(2))
				Expect(servicePorts[0].(map[string]any)["port"]).To(Equal(80))
				Expect(servicePorts[1].(map[string]any)["port"]).To(Equal(8080))
			})
		})

		Describe("Pick behavior via component render", func() {
			It("should only include picked fields in output", func() {
				items := defkit.List("items").WithFields(
					defkit.String("name"),
					defkit.String("path"),
					defkit.String("extra"),
					defkit.Int("count"),
				)

				comp := defkit.NewComponent("test").
					Params(items).
					Template(func(tpl *defkit.Template) {
						picked := defkit.Each(items).Pick("name", "path")
						tpl.Output(
							defkit.NewResource("v1", "ConfigMap").
								Set("data.items", picked),
						)
					})

				rendered := comp.Render(
					defkit.TestContext().
						WithParam("items", []map[string]any{
							{"name": "item1", "path": "/path1", "extra": "ignored", "count": 10},
							{"name": "item2", "path": "/path2", "extra": "also ignored", "count": 20},
						}),
				)

				resultItems := rendered.Get("data.items").([]any)
				Expect(resultItems).To(HaveLen(2))

				// Verify only picked fields are present
				item1 := resultItems[0].(map[string]any)
				Expect(item1).To(HaveKey("name"))
				Expect(item1).To(HaveKey("path"))
				Expect(item1).NotTo(HaveKey("extra"))
				Expect(item1).NotTo(HaveKey("count"))
				Expect(item1["name"]).To(Equal("item1"))
				Expect(item1["path"]).To(Equal("/path1"))
			})
		})

		Describe("Rename behavior via component render", func() {
			It("should rename field in all items", func() {
				ports := defkit.List("ports").WithFields(
					defkit.Int("port"),
					defkit.String("name"),
				)

				comp := defkit.NewComponent("test").
					Params(ports).
					Template(func(tpl *defkit.Template) {
						renamedPorts := defkit.Each(ports).Rename("port", "containerPort")
						tpl.Output(
							defkit.NewResource("apps/v1", "Deployment").
								Set("spec.template.spec.containers[0].ports", renamedPorts),
						)
					})

				rendered := comp.Render(
					defkit.TestContext().
						WithParam("ports", []map[string]any{
							{"port": 80, "name": "http"},
							{"port": 443, "name": "https"},
						}),
				)

				containerPorts := rendered.Get("spec.template.spec.containers[0].ports").([]any)
				Expect(containerPorts).To(HaveLen(2))

				// Verify rename
				port1 := containerPorts[0].(map[string]any)
				Expect(port1).To(HaveKey("containerPort"))
				Expect(port1).NotTo(HaveKey("port"))
				Expect(port1["containerPort"]).To(Equal(80))
				Expect(port1["name"]).To(Equal("http"))
			})
		})

		Describe("Wrap behavior via component render", func() {
			It("should wrap each item value under specified key", func() {
				secrets := defkit.StringList("imagePullSecrets")

				comp := defkit.NewComponent("test").
					Params(secrets).
					Template(func(tpl *defkit.Template) {
						wrappedSecrets := defkit.Each(secrets).Wrap("name")
						tpl.Output(
							defkit.NewResource("apps/v1", "Deployment").
								Set("spec.template.spec.imagePullSecrets", wrappedSecrets),
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

		Describe("DefaultField behavior via component render", func() {
			It("should set default value for missing fields", func() {
				ports := defkit.List("ports").WithFields(
					defkit.Int("port"),
					defkit.String("name"),
					defkit.String("protocol"),
				)

				comp := defkit.NewComponent("test").
					Params(ports).
					Template(func(tpl *defkit.Template) {
						portsWithDefaults := defkit.Each(ports).
							DefaultField("protocol", defkit.LitField("TCP"))
						tpl.Output(
							defkit.NewResource("apps/v1", "Deployment").
								Set("spec.template.spec.containers[0].ports", portsWithDefaults),
						)
					})

				rendered := comp.Render(
					defkit.TestContext().
						WithParam("ports", []map[string]any{
							{"port": 80, "name": "http"},                      // no protocol - should get default
							{"port": 443, "name": "https", "protocol": "TCP"}, // has protocol - keep it
							{"port": 53, "name": "dns", "protocol": "UDP"},    // has different protocol - keep it
						}),
				)

				containerPorts := rendered.Get("spec.template.spec.containers[0].ports").([]any)
				Expect(containerPorts).To(HaveLen(3))
				Expect(containerPorts[0].(map[string]any)["protocol"]).To(Equal("TCP"))
				Expect(containerPorts[1].(map[string]any)["protocol"]).To(Equal("TCP"))
				Expect(containerPorts[2].(map[string]any)["protocol"]).To(Equal("UDP"))
			})
		})

		Describe("Map behavior via component render", func() {
			It("should transform items according to field mappings", func() {
				ports := defkit.List("ports").WithFields(
					defkit.Int("port"),
					defkit.String("name"),
					defkit.String("protocol"),
				)

				comp := defkit.NewComponent("test").
					Params(ports).
					Template(func(tpl *defkit.Template) {
						mappedPorts := defkit.Each(ports).Map(defkit.FieldMap{
							"containerPort": defkit.FieldRef("port"),
							"portName":      defkit.FieldRef("name"),
							"proto":         defkit.FieldRef("protocol"),
						})
						tpl.Output(
							defkit.NewResource("apps/v1", "Deployment").
								Set("spec.template.spec.containers[0].ports", mappedPorts),
						)
					})

				rendered := comp.Render(
					defkit.TestContext().
						WithParam("ports", []map[string]any{
							{"port": 80, "name": "http", "protocol": "TCP"},
						}),
				)

				containerPorts := rendered.Get("spec.template.spec.containers[0].ports").([]any)
				Expect(containerPorts).To(HaveLen(1))

				port := containerPorts[0].(map[string]any)
				Expect(port["containerPort"]).To(Equal(80))
				Expect(port["portName"]).To(Equal("http"))
				Expect(port["proto"]).To(Equal("TCP"))
				// Original keys should not be present
				Expect(port).NotTo(HaveKey("port"))
				Expect(port).NotTo(HaveKey("name"))
				Expect(port).NotTo(HaveKey("protocol"))
			})
		})

		Describe("Chained operations behavior via component render", func() {
			It("should apply all operations in sequence correctly", func() {
				ports := defkit.List("ports").WithFields(
					defkit.Int("port"),
					defkit.String("name"),
					defkit.Bool("expose").Default(false),
				)

				comp := defkit.NewComponent("test").
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
							{"port": 80, "name": "http", "expose": true},
							{"port": 443, "name": "https", "expose": false},
							{"port": 8080, "name": "admin", "expose": true},
						}),
				)

				servicePorts := rendered.Get("spec.ports").([]any)
				Expect(servicePorts).To(HaveLen(2)) // Only exposed ports

				port1 := servicePorts[0].(map[string]any)
				Expect(port1["port"]).To(Equal(80))
				Expect(port1["targetPort"]).To(Equal(80))
				Expect(port1["name"]).To(Equal("http"))
				Expect(port1).NotTo(HaveKey("expose")) // Not in mapping

				port2 := servicePorts[1].(map[string]any)
				Expect(port2["port"]).To(Equal(8080))
				Expect(port2["targetPort"]).To(Equal(8080))
				Expect(port2["name"]).To(Equal("admin"))
			})
		})
	})
})
