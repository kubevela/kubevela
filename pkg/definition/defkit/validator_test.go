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

var _ = Describe("Validator", func() {
	var gen *defkit.CUEGenerator

	BeforeEach(func() {
		gen = defkit.NewCUEGenerator()
	})

	// --- Validator builder and accessors ---

	Context("Builder and Accessors", func() {
		It("should create a validator with message", func() {
			v := defkit.Validate("field is required")
			Expect(v.Message()).To(Equal("field is required"))
			Expect(v.CUEName()).To(BeEmpty())
			Expect(v.GuardCondition()).To(BeNil())
			Expect(v.FailCondition()).To(BeNil())
		})

		It("should set and return WithName", func() {
			v := defkit.Validate("msg").WithName("_validateFoo")
			Expect(v.CUEName()).To(Equal("_validateFoo"))
		})

		It("should set and return FailWhen condition", func() {
			cond := defkit.LocalField("x").Eq("")
			v := defkit.Validate("msg").FailWhen(cond)
			Expect(v.FailCondition()).To(Equal(cond))
		})

		It("should set and return OnlyWhen guard condition", func() {
			guard := defkit.Bool("flag").Eq(true)
			v := defkit.Validate("msg").OnlyWhen(guard)
			Expect(v.GuardCondition()).To(Equal(guard))
		})

		It("should support full builder chain returning all accessors correctly", func() {
			guard := defkit.Bool("x").Eq(true)
			fail := defkit.LocalField("y").Eq("")
			v := defkit.Validate("y is required").
				WithName("_validateY").
				OnlyWhen(guard).
				FailWhen(fail)

			Expect(v.Message()).To(Equal("y is required"))
			Expect(v.CUEName()).To(Equal("_validateY"))
			Expect(v.GuardCondition()).To(Equal(guard))
			Expect(v.FailCondition()).To(Equal(fail))
		})
	})

	// --- Validator CUE generation ---

	Context("Unguarded Validator CUE Generation", func() {
		It("should generate a basic unguarded validator block", func() {
			v := defkit.Validate("tenantName must not end with a hyphen").
				WithName("_validateTenantName").
				FailWhen(defkit.LocalField("tenantName").Matches(".*-$"))

			comp := defkit.NewComponent("test").
				Params(defkit.String("tenantName")).
				Validators(v)

			cue := gen.GenerateParameterSchema(comp)

			Expect(cue).To(ContainSubstring("_validateTenantName:"))
			Expect(cue).To(ContainSubstring(`"tenantName must not end with a hyphen": true`))
			Expect(cue).To(ContainSubstring(`tenantName =~ ".*-$"`))
			Expect(cue).To(ContainSubstring(`"tenantName must not end with a hyphen": false`))
		})

		It("should use fallback _validate name when no name is set", func() {
			v := defkit.Validate("something is wrong")
			comp := defkit.NewComponent("test").Validators(v)
			cue := gen.GenerateParameterSchema(comp)

			Expect(cue).To(ContainSubstring("_validate:"))
		})

		It("should generate validator with no fail condition", func() {
			v := defkit.Validate("always passes").WithName("_validateAlways")
			comp := defkit.NewComponent("test").Validators(v)
			cue := gen.GenerateParameterSchema(comp)

			Expect(cue).To(ContainSubstring(`_validateAlways:`))
			Expect(cue).To(ContainSubstring(`"always passes": true`))
			Expect(cue).NotTo(ContainSubstring(`"always passes": false`))
		})
	})

	Context("Guarded Validator CUE Generation", func() {
		It("should wrap validator in guard condition", func() {
			replConfig := defkit.Object("replicationConfiguration").Optional()
			objectLock := defkit.Object("objectLock").Optional()
			versioningEnabled := defkit.Bool("versioningEnabled").Default(true)

			v := defkit.Validate("Require versioningEnabled to be true when replication or object lock is configured").
				WithName("_validateVersioning").
				OnlyWhen(defkit.Or(replConfig.IsSet(), objectLock.IsSet())).
				FailWhen(versioningEnabled.Eq(false))

			comp := defkit.NewComponent("test").
				Params(replConfig, objectLock, versioningEnabled).
				Validators(v)

			cue := gen.GenerateParameterSchema(comp)

			Expect(cue).To(ContainSubstring(`replicationConfiguration"] != _|_`))
			Expect(cue).To(ContainSubstring(`objectLock"] != _|_`))
			Expect(cue).To(ContainSubstring("parameter.versioningEnabled == false"))
			Expect(cue).To(ContainSubstring("_validateVersioning:"))
		})
	})

	Context("Validator inside MapParam", func() {
		It("should emit validator inside struct", func() {
			v := defkit.Validate("name is required").
				WithName("_validateName").
				FailWhen(defkit.LocalField("name").Eq(""))

			mp := defkit.Object("governance").WithFields(
				defkit.String("name"),
				defkit.String("department"),
			).Validators(v)

			comp := defkit.NewComponent("test").Params(mp)
			cue := gen.GenerateParameterSchema(comp)

			Expect(cue).To(ContainSubstring("governance: {"))
			Expect(cue).To(ContainSubstring("_validateName:"))
			Expect(cue).To(ContainSubstring(`name == ""`))
		})

		It("should emit mutual exclusion validator inside struct", func() {
			v := defkit.Validate("Principal and NotPrincipal cannot be used together").
				WithName("_validatePrincipal").
				FailWhen(defkit.And(defkit.LocalField("Principal").IsSet(), defkit.LocalField("NotPrincipal").IsSet()))

			comp := defkit.NewComponent("test").
				Params(
					defkit.Object("statement").WithFields(
						defkit.String("Principal").Optional(),
						defkit.String("NotPrincipal").Optional(),
					).Validators(v),
				)

			cue := gen.GenerateParameterSchema(comp)

			Expect(cue).To(ContainSubstring("_validatePrincipal:"))
			Expect(cue).To(ContainSubstring("Principal != _|_"))
			Expect(cue).To(ContainSubstring("NotPrincipal != _|_"))
		})
	})

	Context("Validator inside ArrayParam", func() {
		It("should emit validator inside array element struct", func() {
			v := defkit.Validate("action is required").
				WithName("_validateAction").
				FailWhen(defkit.LocalField("action").Eq(""))

			arr := defkit.Array("rules").WithFields(
				defkit.String("action"),
				defkit.String("resource"),
			).Validators(v)

			comp := defkit.NewComponent("test").Params(arr)
			cue := gen.GenerateParameterSchema(comp)

			Expect(cue).To(ContainSubstring("[...{"))
			Expect(cue).To(ContainSubstring("_validateAction:"))
		})

		It("should emit non-empty length validator inside array element struct", func() {
			arr := defkit.Array("corsRules").Optional().WithFields(
				defkit.Array("allowedMethods").OfEnum("GET", "PUT", "HEAD", "POST", "DELETE"),
			).Validators(
				defkit.Validate("allowedMethods cannot be empty").
					WithName("_validateAllowedMethods").
					FailWhen(defkit.LocalField("allowedMethods").IsEmpty()),
			)

			comp := defkit.NewComponent("test").Params(arr)
			cue := gen.GenerateParameterSchema(comp)

			Expect(cue).To(ContainSubstring("len(allowedMethods) == 0"))
		})
	})

	// --- LocalFieldRef ---

	Context("LocalFieldRef", func() {
		It("should return the field name", func() {
			ref := defkit.LocalField("tenantName")
			Expect(ref.Name()).To(Equal("tenantName"))
		})

		It("should support dot-path names", func() {
			ref := defkit.LocalField("Principal.AWS")
			Expect(ref.Name()).To(Equal("Principal.AWS"))
		})

		It("should support array-indexed names", func() {
			ref := defkit.LocalField("expiration[0].date")
			Expect(ref.Name()).To(Equal("expiration[0].date"))
		})

		It("should implement Value interface", func() {
			ref := defkit.LocalField("test")
			var v defkit.Value = ref
			Expect(v).NotTo(BeNil())
		})

		It("Matches should generate regex match CUE condition", func() {
			v := defkit.Validate("bad pattern").
				WithName("_v").
				FailWhen(defkit.LocalField("name").Matches(".*-$"))

			comp := defkit.NewComponent("test").Validators(v)
			cue := gen.GenerateParameterSchema(comp)
			Expect(cue).To(ContainSubstring(`name =~ ".*-$"`))
		})

		It("Eq should generate equality CUE condition", func() {
			v := defkit.Validate("check").
				WithName("_v").
				FailWhen(defkit.LocalField("type").Eq("disabled"))

			comp := defkit.NewComponent("test").Validators(v)
			cue := gen.GenerateParameterSchema(comp)
			Expect(cue).To(ContainSubstring(`type == "disabled"`))
		})

		It("Ne should generate inequality CUE condition", func() {
			v := defkit.Validate("check").
				WithName("_v").
				FailWhen(defkit.LocalField("type").Ne("enabled"))

			comp := defkit.NewComponent("test").Validators(v)
			cue := gen.GenerateParameterSchema(comp)
			Expect(cue).To(ContainSubstring(`type != "enabled"`))
		})

		It("IsSet should generate path-exists CUE condition", func() {
			v := defkit.Validate("check").
				WithName("_v").
				FailWhen(defkit.Not(defkit.LocalField("role").IsSet()))

			comp := defkit.NewComponent("test").Validators(v)
			cue := gen.GenerateParameterSchema(comp)
			Expect(cue).To(ContainSubstring("role == _|_"))
		})

		It("NotSet should generate path-not-exists CUE condition", func() {
			v := defkit.Validate("role must be set").
				WithName("_v").
				FailWhen(defkit.LocalField("role").NotSet())

			comp := defkit.NewComponent("test").Validators(v)
			cue := gen.GenerateParameterSchema(comp)
			Expect(cue).To(ContainSubstring("role == _|_"))
		})

		It("LenEq should generate length equality CUE condition", func() {
			v := defkit.Validate("check").
				WithName("_v").
				FailWhen(defkit.LocalField("Principal.AWS").LenEq(0))

			comp := defkit.NewComponent("test").Validators(v)
			cue := gen.GenerateParameterSchema(comp)
			Expect(cue).To(ContainSubstring("len(Principal.AWS) == 0"))
		})

		It("LenGt should generate length greater-than CUE condition", func() {
			v := defkit.Validate("too many").
				WithName("_v").
				FailWhen(defkit.LocalField("items").LenGt(10))

			comp := defkit.NewComponent("test").Validators(v)
			cue := gen.GenerateParameterSchema(comp)
			Expect(cue).To(ContainSubstring("len(items) > 10"))
		})

		It("IsEmpty should generate length == 0 CUE condition", func() {
			v := defkit.Validate("empty").
				WithName("_v").
				FailWhen(defkit.LocalField("list").IsEmpty())

			comp := defkit.NewComponent("test").Validators(v)
			cue := gen.GenerateParameterSchema(comp)
			Expect(cue).To(ContainSubstring("len(list) == 0"))
		})

		It("Gte should generate >= comparison between two local fields", func() {
			v := defkit.Validate("days must be >= minDays").
				WithName("_v").
				FailWhen(defkit.Not(defkit.LocalField("days").Gte(defkit.LocalField("minDays"))))

			comp := defkit.NewComponent("test").Validators(v)
			cue := gen.GenerateParameterSchema(comp)
			Expect(cue).To(ContainSubstring("days >= minDays"))
		})
	})

	// --- LenOfExpr ---

	Context("LenOfExpr", func() {
		It("Gt should generate len(value) > n", func() {
			v := defkit.Validate("too long").
				WithName("_v").
				FailWhen(defkit.LenOf(defkit.LocalField("name")).Gt(63))

			comp := defkit.NewComponent("test").Validators(v)
			cue := gen.GenerateParameterSchema(comp)
			Expect(cue).To(ContainSubstring("len(name) > 63"))
		})

		It("Gte should generate len(value) >= n", func() {
			v := defkit.Validate("too long").
				WithName("_v").
				FailWhen(defkit.LenOf(defkit.LocalField("data")).Gte(100))

			comp := defkit.NewComponent("test").Validators(v)
			cue := gen.GenerateParameterSchema(comp)
			Expect(cue).To(ContainSubstring("len(data) >= 100"))
		})

		It("Eq should generate len(value) == n", func() {
			v := defkit.Validate("wrong length").
				WithName("_v").
				FailWhen(defkit.Not(defkit.LenOf(defkit.LocalField("code")).Eq(3)))

			comp := defkit.NewComponent("test").Validators(v)
			cue := gen.GenerateParameterSchema(comp)
			Expect(cue).To(ContainSubstring("len(code) == 3"))
		})

		It("should work with complex expressions like Plus", func() {
			v := defkit.Validate("combined name too long").
				WithName("_v").
				FailWhen(defkit.LenOf(defkit.Plus(
					defkit.Lit("tenant-"),
					defkit.Reference("parameter.name"),
				)).Gt(63))

			comp := defkit.NewComponent("test").Validators(v)
			cue := gen.GenerateParameterSchema(comp)
			Expect(cue).To(ContainSubstring(`len("tenant-" + parameter.name) > 63`))
		})
	})

	// --- TimeParse ---

	Context("TimeParse", func() {
		It("should return correct accessors", func() {
			tp := defkit.TimeParse("2006-01-02T15:04:05Z", defkit.LocalField("startDate"))
			Expect(tp.Layout()).To(Equal("2006-01-02T15:04:05Z"))
			Expect(tp.FieldName()).To(Equal("startDate"))
		})

		It("should generate time.Parse CUE expression in validator", func() {
			v := defkit.Validate("start must be before end").
				WithName("_v").
				FailWhen(defkit.TimeParse("2006-01-02T15:04:05Z", defkit.LocalField("startDate")).
					Gte(defkit.TimeParse("2006-01-02T15:04:05Z", defkit.LocalField("endDate"))))

			comp := defkit.NewComponent("test").Validators(v)
			cue := gen.GenerateParameterSchema(comp)

			Expect(cue).To(ContainSubstring(`time.Parse("2006-01-02T15:04:05Z", startDate)`))
			Expect(cue).To(ContainSubstring(`time.Parse("2006-01-02T15:04:05Z", endDate)`))
			Expect(cue).To(ContainSubstring(">="))
		})

		It("should use different layout formats", func() {
			v := defkit.Validate("check").
				WithName("_v").
				FailWhen(defkit.TimeParse("2006-01-02", defkit.LocalField("d1")).
					Gte(defkit.TimeParse("2006-01-02", defkit.LocalField("d2"))))

			comp := defkit.NewComponent("test").Validators(v)
			cue := gen.GenerateParameterSchema(comp)

			Expect(cue).To(ContainSubstring(`time.Parse("2006-01-02", d1)`))
			Expect(cue).To(ContainSubstring(`time.Parse("2006-01-02", d2)`))
		})
	})

	// --- RawCUECondition / CUEExpr ---

	Context("CUEExpr", func() {
		It("should return raw expression from accessor", func() {
			c := defkit.CUEExpr(`len(x) > 5`)
			Expect(c.Expr()).To(Equal(`len(x) > 5`))
		})

		It("should emit raw expression in guarded validator", func() {
			existingResources := defkit.Bool("existingResources").Default(false)

			v := defkit.Validate("Combined name must be less than 64 characters").
				WithName("_validateNameLength").
				OnlyWhen(existingResources.Eq(false)).
				FailWhen(defkit.CUEExpr(`len("tenant-"+parameter.governance.tenantName+"-"+name) > 63`))

			comp := defkit.NewComponent("test").
				Params(existingResources).
				Validators(v)

			cue := gen.GenerateParameterSchema(comp)

			Expect(cue).To(ContainSubstring(`len("tenant-"+parameter.governance.tenantName+"-"+name) > 63`))
			Expect(cue).To(ContainSubstring("if parameter.existingResources == false"))
		})

		It("should work inside And condition", func() {
			v := defkit.Validate("complex check").
				WithName("_validateComplex").
				FailWhen(defkit.And(
					defkit.CUEExpr(`len(parameter.name) > 10`),
					defkit.CUEExpr(`parameter.name =~ "^test"`),
				))

			comp := defkit.NewComponent("test").Validators(v)
			cue := gen.GenerateParameterSchema(comp)

			Expect(cue).To(ContainSubstring("len(parameter.name) > 10"))
			Expect(cue).To(ContainSubstring(`parameter.name =~ "^test"`))
		})
	})

	// --- ConditionalParams CUE generation ---

	Context("ConditionalParams CUE Generation", func() {
		It("should generate conditional parameter blocks", func() {
			existingResources := defkit.Bool("existingResources").Default(false)

			comp := defkit.NewComponent("test").
				Params(existingResources).
				ConditionalParams(defkit.ConditionalParams(
					defkit.WhenParam(existingResources.Eq(false)).Params(
						defkit.Bool("forceDestroy").Default(false),
						defkit.String("sseAlgorithm").Default("AES256").Values("AES256", "aws:kms"),
					),
					defkit.WhenParam(existingResources.Eq(true)).Params(
						defkit.Bool("forceDestroy").Optional(),
						defkit.String("sseAlgorithm").Optional().Values("AES256", "aws:kms"),
					),
				))

			cue := gen.GenerateParameterSchema(comp)

			Expect(cue).To(ContainSubstring("if parameter.existingResources == false"))
			Expect(cue).To(ContainSubstring("if parameter.existingResources == true"))
			Expect(cue).To(ContainSubstring(`forceDestroy: *false | bool`))
			Expect(cue).To(ContainSubstring("forceDestroy?: bool"))
		})

		It("should generate validators inside conditional blocks", func() {
			existingResources := defkit.Bool("existingResources").Default(false)
			kmsMasterKeyId := defkit.String("kmsMasterKeyId").Optional()

			comp := defkit.NewComponent("test").
				Params(existingResources, kmsMasterKeyId).
				ConditionalParams(defkit.ConditionalParams(
					defkit.WhenParam(existingResources.Eq(false)).Params(
						defkit.String("sseAlgorithm").Default("AES256").Values("AES256", "aws:kms"),
					).Validators(
						defkit.Validate("kmsMasterKeyId can only be specified when sseAlgorithm is aws:kms").
							WithName("_validateKms").
							FailWhen(defkit.And(
								defkit.LocalField("sseAlgorithm").Ne("aws:kms"),
								kmsMasterKeyId.IsSet(),
							)),
					),
				))

			cue := gen.GenerateParameterSchema(comp)

			Expect(cue).To(ContainSubstring("_validateKms:"))
			Expect(cue).To(ContainSubstring(`sseAlgorithm != "aws:kms"`))
		})
	})

	Context("ConditionalFields Inside MapParam CUE Generation", func() {
		It("should generate conditional fields inside struct", func() {
			existingResources := defkit.Bool("existingResources").Default(false)

			objectLock := defkit.Object("objectLock").Optional().ConditionalFields(
				defkit.WhenParam(existingResources.Eq(false)).Params(
					defkit.Int("retentionDays").Optional().Default(45).Min(1),
					defkit.String("retentionMode").Optional().Default("GOVERNANCE").Values("GOVERNANCE", "COMPLIANCE"),
				),
				defkit.WhenParam(existingResources.Eq(true)).Params(
					defkit.Int("retentionDays").Min(1),
					defkit.String("retentionMode").Values("GOVERNANCE", "COMPLIANCE"),
				),
			)

			comp := defkit.NewComponent("test").Params(existingResources, objectLock)
			cue := gen.GenerateParameterSchema(comp)

			Expect(cue).To(ContainSubstring("objectLock?: {"))
			Expect(cue).To(ContainSubstring("if parameter.existingResources == false"))
			Expect(cue).To(ContainSubstring(`retentionDays?: *45 | int & >=1`))
		})
	})

	// --- ConditionalStruct on Resource CUE generation ---

	Context("ConditionalStruct CUE Generation", func() {
		It("should generate conditional struct block in output", func() {
			replConfig := defkit.Object("replicationConfiguration").Optional()

			comp := defkit.NewComponent("test").
				Params(replConfig).
				Workload("apps/v1", "Deployment").
				Template(func(tpl *defkit.Template) {
					output := tpl.Output(defkit.NewResource("v1", "ConfigMap"))
					output.Set("metadata.name", defkit.Lit("test")).
						ConditionalStruct(replConfig.IsSet(), "spec.replicationConfiguration", func(b *defkit.OutputStructBuilder) {
							b.Set("role", defkit.Reference("parameter.replicationConfiguration.role"))
							b.SetIf(replConfig.IsSet(), "enabled", defkit.Lit(true))
						})
				})

			cue := gen.GenerateTemplate(comp)

			Expect(cue).To(ContainSubstring(`if parameter["replicationConfiguration"] != _|_`))
			Expect(cue).To(ContainSubstring("replicationConfiguration:"))
			Expect(cue).To(ContainSubstring("role: parameter.replicationConfiguration.role"))
			Expect(cue).To(ContainSubstring("enabled:"))
		})

		It("should generate conditional struct with SetIf inside", func() {
			existingResources := defkit.Bool("existingResources").Default(false)
			replConfig := defkit.Object("replicationConfiguration").Optional()

			comp := defkit.NewComponent("test").
				Params(existingResources, replConfig).
				Workload("apps/v1", "Deployment").
				Template(func(tpl *defkit.Template) {
					output := tpl.Output(defkit.NewResource("v1", "ConfigMap"))
					output.Set("metadata.name", defkit.Lit("test")).
						ConditionalStruct(replConfig.IsSet(), "spec.replication", func(b *defkit.OutputStructBuilder) {
							b.Set("role", defkit.Reference("parameter.replicationConfiguration.role"))
							b.SetIf(existingResources.Eq(false), "destinationBucketName", defkit.Lit("replica-bucket"))
						})
				})

			cue := gen.GenerateTemplate(comp)

			Expect(cue).To(ContainSubstring("parameter.existingResources == false"))
			Expect(cue).To(ContainSubstring("destinationBucketName:"))
		})
	})

	// --- Integration: all features combined ---

	Context("All Features Integrated", func() {
		It("should combine validators, conditional params, closed structs, and CUE expressions", func() {
			existingResources := defkit.Bool("existingResources").Default(false)
			governance := defkit.Object("governance").Closed().WithFields(
				defkit.String("tenantName").NotEmpty(),
				defkit.String("departmentCode").NotEmpty(),
			).Validators(
				defkit.Validate("tenantName must not end with a hyphen").
					WithName("_validateTenant").
					FailWhen(defkit.LocalField("tenantName").Matches(".*-$")),
			)

			comp := defkit.NewComponent("s3-bucket").
				Params(existingResources, governance).
				ConditionalParams(defkit.ConditionalParams(
					defkit.WhenParam(existingResources.Eq(false)).Params(
						defkit.Bool("forceDestroy").Default(false),
					),
					defkit.WhenParam(existingResources.Eq(true)).Params(
						defkit.Bool("forceDestroy").Optional(),
					),
				)).
				Validators(
					defkit.Validate("Combined name check").
						WithName("_validateName").
						OnlyWhen(existingResources.Eq(false)).
						FailWhen(defkit.CUEExpr(`len(parameter.governance.tenantName) > 63`)),
				)

			cue := gen.GenerateParameterSchema(comp)

			Expect(cue).To(ContainSubstring("existingResources: *false | bool"))
			Expect(cue).To(ContainSubstring("governance: close({"))
			Expect(cue).To(ContainSubstring(`!=""`))
			// Validator uses Matches which emits =~ (positive match inside fail block)
			Expect(cue).To(ContainSubstring(`tenantName =~ ".*-$"`))
			Expect(cue).To(ContainSubstring("_validateTenant:"))
			Expect(cue).To(ContainSubstring("if parameter.existingResources == false"))
			Expect(cue).To(ContainSubstring("if parameter.existingResources == true"))
			Expect(cue).To(ContainSubstring("forceDestroy: *false | bool"))
			Expect(cue).To(ContainSubstring("forceDestroy?: bool"))
			Expect(cue).To(ContainSubstring("_validateName:"))
			Expect(cue).To(ContainSubstring(`len(parameter.governance.tenantName) > 63`))
		})
	})
})
