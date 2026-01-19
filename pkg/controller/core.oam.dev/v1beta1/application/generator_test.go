/*Copyright 2021 The KubeVela Authors.

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

package application

import (
	"context"
	"encoding/json"
	"strconv"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/yaml"

	monitorContext "github.com/kubevela/pkg/monitor/context"
	workflowv1alpha1 "github.com/kubevela/workflow/api/v1alpha1"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	oamcore "github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/oam/util"
)

var _ = Describe("Test Application workflow generator", func() {
	var namespaceName string
	var ns corev1.Namespace
	var ctx context.Context

	BeforeEach(func() {
		namespaceName = "generate-test-" + strconv.Itoa(time.Now().Second()) + "-" + strconv.Itoa(time.Now().Nanosecond())
		ctx = context.WithValue(context.TODO(), util.AppDefinitionNamespace, namespaceName)
		ns = corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: namespaceName,
			},
		}
		By("Create the Namespace for test")
		Expect(k8sClient.Create(ctx, &ns)).Should(Succeed())

		healthComponentDef := &oamcore.ComponentDefinition{}
		hCDefJson, _ := yaml.YAMLToJSON([]byte(cdDefWithHealthStatusYaml))
		Expect(json.Unmarshal(hCDefJson, healthComponentDef)).Should(BeNil())
		healthComponentDef.Name = "worker-with-health"
		healthComponentDef.Namespace = namespaceName

		By("Create the Component Definition for test")
		Expect(k8sClient.Create(ctx, healthComponentDef)).Should(Succeed())
	})

	AfterEach(func() {
		By("[TEST] Clean up resources after an integration test")
		Expect(k8sClient.Delete(context.TODO(), &ns)).Should(Succeed())
	})

	It("Test generate application workflow with inputs and outputs", func() {
		app := &oamcore.Application{
			TypeMeta: metav1.TypeMeta{
				Kind:       "Application",
				APIVersion: "core.oam.dev/v1beta1",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      "app-with-input-output",
				Namespace: namespaceName,
			},
			Spec: oamcore.ApplicationSpec{
				Components: []common.ApplicationComponent{
					{
						Name:       "myweb1",
						Type:       "worker-with-health",
						Properties: &runtime.RawExtension{Raw: []byte(`{"cmd":["sleep","1000"],"image":"busybox"}`)},
						Inputs: workflowv1alpha1.StepInputs{
							{
								From:         "message",
								ParameterKey: "properties.enemies",
							},
							{
								From:         "message",
								ParameterKey: "properties.lives",
							},
						},
					},
					{
						Name:       "myweb2",
						Type:       "worker-with-health",
						Properties: &runtime.RawExtension{Raw: []byte(`{"cmd":["sleep","1000"],"image":"busybox","lives": "i am lives","enemies": "empty"}`)},
						Outputs: workflowv1alpha1.StepOutputs{
							{
								Name:      "message",
								ValueFrom: "output.status.conditions[0].message+\",\"+outputs.gameconfig.data.lives",
							},
						},
					},
				},
			},
		}
		af, err := appParser.GenerateAppFile(ctx, app)
		Expect(err).Should(BeNil())
		_, err = af.GeneratePolicyManifests(context.Background())
		Expect(err).Should(BeNil())

		handler, err := NewAppHandler(ctx, reconciler, app)
		Expect(err).Should(Succeed())

		logCtx := monitorContext.NewTraceContext(ctx, "")
		handler.currentAppRev = &oamcore.ApplicationRevision{}

		_, taskRunner, err := handler.GenerateApplicationSteps(logCtx, app, appParser, af)
		Expect(err).To(BeNil())
		Expect(len(taskRunner)).Should(BeEquivalentTo(2))
		Expect(taskRunner[0].Name()).Should(BeEquivalentTo("myweb1"))
		Expect(taskRunner[1].Name()).Should(BeEquivalentTo("myweb2"))
	})

	It("Test generate application workflow without inputs and outputs", func() {
		app := &oamcore.Application{
			TypeMeta: metav1.TypeMeta{
				Kind:       "Application",
				APIVersion: "core.oam.dev/v1beta1",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      "app-without-input-output",
				Namespace: namespaceName,
			},
			Spec: oamcore.ApplicationSpec{
				Components: []common.ApplicationComponent{
					{
						Name:       "myweb1",
						Type:       "worker-with-health",
						Properties: &runtime.RawExtension{Raw: []byte(`{"cmd":["sleep","1000"],"image":"busybox"}`)},
					},
					{
						Name:       "myweb2",
						Type:       "worker-with-health",
						Properties: &runtime.RawExtension{Raw: []byte(`{"cmd":["sleep","1000"],"image":"busybox","lives": "i am lives","enemies": "empty"}`)},
					},
				},
			},
		}
		af, err := appParser.GenerateAppFile(ctx, app)
		Expect(err).Should(BeNil())
		_, err = af.GeneratePolicyManifests(context.Background())
		Expect(err).Should(BeNil())

		handler, err := NewAppHandler(ctx, reconciler, app)
		Expect(err).Should(Succeed())

		logCtx := monitorContext.NewTraceContext(ctx, "")
		handler.currentAppRev = &oamcore.ApplicationRevision{}
		_, taskRunner, err := handler.GenerateApplicationSteps(logCtx, app, appParser, af)
		Expect(err).To(BeNil())
		Expect(len(taskRunner)).Should(BeEquivalentTo(2))
		Expect(taskRunner[0].Name()).Should(BeEquivalentTo("myweb1"))
		Expect(taskRunner[1].Name()).Should(BeEquivalentTo("myweb2"))
	})

	It("Test generate application workflow with dependsOn", func() {
		app := &oamcore.Application{
			TypeMeta: metav1.TypeMeta{
				Kind:       "Application",
				APIVersion: "core.oam.dev/v1beta1",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      "app-with-input-output",
				Namespace: namespaceName,
			},
			Spec: oamcore.ApplicationSpec{
				Components: []common.ApplicationComponent{
					{
						Name:       "myweb1",
						Type:       "worker-with-health",
						Properties: &runtime.RawExtension{Raw: []byte(`{"cmd":["sleep","1000"],"image":"busybox"}`)},
					},
					{
						Name:       "myweb2",
						Type:       "worker-with-health",
						DependsOn:  []string{"myweb1"},
						Properties: &runtime.RawExtension{Raw: []byte(`{"cmd":["sleep","1000"],"image":"busybox","lives": "i am lives","enemies": "empty"}`)},
					},
				},
			},
		}
		af, err := appParser.GenerateAppFile(ctx, app)
		Expect(err).Should(BeNil())

		handler, err := NewAppHandler(ctx, reconciler, app)
		Expect(err).Should(Succeed())

		logCtx := monitorContext.NewTraceContext(ctx, "")
		handler.currentAppRev = &oamcore.ApplicationRevision{}
		_, taskRunner, err := handler.GenerateApplicationSteps(logCtx, app, appParser, af)
		Expect(err).To(BeNil())
		Expect(len(taskRunner)).Should(BeEquivalentTo(2))
		Expect(taskRunner[0].Name()).Should(BeEquivalentTo("myweb1"))
		Expect(taskRunner[1].Name()).Should(BeEquivalentTo("myweb2"))
	})

	It("Test generate application workflow with invalid dependsOn", func() {
		app := &oamcore.Application{
			TypeMeta: metav1.TypeMeta{
				Kind:       "Application",
				APIVersion: "core.oam.dev/v1beta1",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      "app-with-input-output",
				Namespace: namespaceName,
			},
			Spec: oamcore.ApplicationSpec{
				Components: []common.ApplicationComponent{
					{
						Name:       "myweb1",
						Type:       "worker-with-health",
						Properties: &runtime.RawExtension{Raw: []byte(`{"cmd":["sleep","1000"],"image":"busybox"}`)},
					},
					{
						Name:       "myweb2",
						Type:       "worker-with-health",
						DependsOn:  []string{"myweb0"},
						Properties: &runtime.RawExtension{Raw: []byte(`{"cmd":["sleep","1000"],"image":"busybox","lives": "i am lives","enemies": "empty"}`)},
					},
				},
			},
		}
		af, err := appParser.GenerateAppFile(ctx, app)
		Expect(err).Should(BeNil())

		handler, err := NewAppHandler(ctx, reconciler, app)
		Expect(err).Should(Succeed())

		logCtx := monitorContext.NewTraceContext(ctx, "")
		handler.currentAppRev = &oamcore.ApplicationRevision{}
		_, _, err = handler.GenerateApplicationSteps(logCtx, app, appParser, af)
		Expect(err).NotTo(BeNil())
	})

	It("Test generate application workflow with multiple invalid dependsOn", func() {
		app := &oamcore.Application{
			TypeMeta: metav1.TypeMeta{
				Kind:       "Application",
				APIVersion: "core.oam.dev/v1beta1",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      "app-with-input-output",
				Namespace: namespaceName,
			},
			Spec: oamcore.ApplicationSpec{
				Components: []common.ApplicationComponent{
					{
						Name:       "myweb1",
						Type:       "worker-with-health",
						Properties: &runtime.RawExtension{Raw: []byte(`{"cmd":["sleep","1000"],"image":"busybox"}`)},
					},
					{
						Name:       "myweb2",
						Type:       "worker-with-health",
						DependsOn:  []string{"myweb1", "myweb0", "myweb3"},
						Properties: &runtime.RawExtension{Raw: []byte(`{"cmd":["sleep","1000"],"image":"busybox","lives": "i am lives","enemies": "empty"}`)},
					},
				},
			},
		}
		af, err := appParser.GenerateAppFile(ctx, app)
		Expect(err).Should(BeNil())

		handler, err := NewAppHandler(ctx, reconciler, app)
		Expect(err).Should(Succeed())

		logCtx := monitorContext.NewTraceContext(ctx, "")
		handler.currentAppRev = &oamcore.ApplicationRevision{}
		_, _, err = handler.GenerateApplicationSteps(logCtx, app, appParser, af)
		Expect(err).NotTo(BeNil())
	})

	It("Test workflow context contains app labels and annotations", func() {
		app := &oamcore.Application{
			TypeMeta: metav1.TypeMeta{Kind: "Application", APIVersion: "core.oam.dev/v1beta1"},
			ObjectMeta: metav1.ObjectMeta{
				Name:        "app-with-meta",
				Namespace:   namespaceName,
				Labels:      map[string]string{"team": "platform", "env": "prod"},
				Annotations: map[string]string{"description": "meta test", "owner": "sre"},
			},
			Spec: oamcore.ApplicationSpec{Components: []common.ApplicationComponent{}},
		}
		ctxData := generateContextDataFromApp(app, "apprev-with-meta")
		Expect(ctxData.AppLabels).To(Equal(app.Labels))
		Expect(ctxData.AppAnnotations).To(Equal(app.Annotations))
	})

	It("Test workflow context empty labels annotations", func() {
		app := &oamcore.Application{
			TypeMeta:   metav1.TypeMeta{Kind: "Application", APIVersion: "core.oam.dev/v1beta1"},
			ObjectMeta: metav1.ObjectMeta{Name: "app-without-meta", Namespace: namespaceName},
			Spec:       oamcore.ApplicationSpec{Components: []common.ApplicationComponent{}},
		}
		ctxData := generateContextDataFromApp(app, "apprev-without-meta")
		Expect(ctxData.AppLabels).To(BeNil())
		Expect(ctxData.AppAnnotations).To(BeNil())
	})

	// NOTE: Workflow restart tests have been migrated to workflow_restart_test.go
	// They test the new annotation-based restart functionality:
	// - handleWorkflowRestartAnnotation() - parses annotation and sets status field
	// - checkWorkflowRestart() - triggers restart based on status field

	/*
		// Original tests commented out below for reference - DO NOT UNCOMMENT
		// See workflow_restart_test.go for the new tests
		/*
		It("Test workflow restart via annotation with immediate restart", func() {
			// Use a past timestamp for immediate restart
			pastTime := time.Now().Add(-1 * time.Hour)
			pastTimeStr := pastTime.Format(time.RFC3339)

			app := &oamcore.Application{
				TypeMeta: metav1.TypeMeta{Kind: "Application", APIVersion: "core.oam.dev/v1beta1"},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "app-with-restart-annotation",
					Namespace: namespaceName,
					Annotations: map[string]string{
						oam.AnnotationWorkflowRestart: pastTimeStr, // Past timestamp = immediate
					},
				},
				Spec: oamcore.ApplicationSpec{
					Components: []common.ApplicationComponent{
						{
							Name:       "myweb",
							Type:       "worker-with-health",
							Properties: &runtime.RawExtension{Raw: []byte(`{"cmd":["sleep","1000"],"image":"busybox"}`)},
						},
					},
				},
				Status: common.AppStatus{
					Workflow: &common.WorkflowStatus{
						AppRevision: "app-with-restart-annotation-v1",
						Finished:    true,
					},
					Services: []common.ApplicationComponentStatus{
						{Name: "myweb", Healthy: true},
					},
				},
			}

			handler, err := NewAppHandler(ctx, reconciler, app)
			Expect(err).Should(Succeed())

			appRev := &oamcore.ApplicationRevision{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "app-with-restart-annotation-v2",
					Namespace: namespaceName,
				},
			}
			handler.currentAppRev = appRev
			handler.latestAppRev = appRev

			logCtx := monitorContext.NewTraceContext(ctx, "")

			// Before annotation handling
			Expect(app.Annotations).To(HaveKey(oam.AnnotationWorkflowRestart))
			Expect(app.Status.WorkflowRestartScheduledAt).To(BeNil())
			Expect(app.Status.Workflow).NotTo(BeNil())
			Expect(app.Status.Workflow.Finished).To(BeTrue())
			Expect(app.Status.Services).To(HaveLen(1))

			// Simulate controller processing annotation - sets status field (annotation removed by controller)
			app.Status.WorkflowRestartScheduledAt = &metav1.Time{Time: pastTime}
			delete(app.Annotations, oam.AnnotationWorkflowRestart)

			// Status field set, annotation removed (done by controller)
			Expect(app.Annotations).NotTo(HaveKey(oam.AnnotationWorkflowRestart))
			Expect(app.Status.WorkflowRestartScheduledAt).NotTo(BeNil())
			Expect(app.Status.WorkflowRestartScheduledAt.Time).To(BeTemporally("~", pastTime, 1*time.Second))

			// Check workflow restart - should trigger restart because time has passed
			handler.CheckWorkflowRestart(logCtx, app)

			// After restart - status field cleared, workflow restarted
			Expect(app.Status.WorkflowRestartScheduledAt).To(BeNil()) // Status field cleared
			Expect(app.Status.Workflow).NotTo(BeNil())
			Expect(app.Status.Workflow.AppRevision).To(Equal("app-with-restart-annotation-v2"))
			Expect(app.Status.Workflow.Finished).To(BeFalse()) // Workflow reset
			Expect(app.Status.Services).To(BeNil())             // Services cleared
		})

		It("Test workflow restart via annotation with past timestamp", func() {
			pastTime := time.Now().Add(-1 * time.Hour)
			pastTimeStr := pastTime.Format(time.RFC3339)
			app := &oamcore.Application{
				TypeMeta: metav1.TypeMeta{Kind: "Application", APIVersion: "core.oam.dev/v1beta1"},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "app-with-past-timestamp",
					Namespace: namespaceName,
					Annotations: map[string]string{
						oam.AnnotationWorkflowRestart: pastTimeStr,
					},
				},
				Spec: oamcore.ApplicationSpec{
					Components: []common.ApplicationComponent{
						{
							Name:       "myweb",
							Type:       "worker-with-health",
							Properties: &runtime.RawExtension{Raw: []byte(`{"cmd":["sleep","1000"],"image":"busybox"}`)},
						},
					},
				},
				Status: common.AppStatus{
					Workflow: &common.WorkflowStatus{
						AppRevision: "app-v1",
						Finished:    true,
					},
				},
			}

			handler, err := NewAppHandler(ctx, reconciler, app)
			Expect(err).Should(Succeed())

			appRev := &oamcore.ApplicationRevision{
				ObjectMeta: metav1.ObjectMeta{Name: "app-v2", Namespace: namespaceName},
			}
			handler.currentAppRev = appRev
			handler.latestAppRev = appRev

			logCtx := monitorContext.NewTraceContext(ctx, "")

			// Simulate controller processing annotation
			app.Status.WorkflowRestartScheduledAt = &metav1.Time{Time: pastTime}
			delete(app.Annotations, oam.AnnotationWorkflowRestart)

			Expect(app.Annotations).NotTo(HaveKey(oam.AnnotationWorkflowRestart))  // Annotation removed
			Expect(app.Status.WorkflowRestartScheduledAt).NotTo(BeNil())            // Status field set
			Expect(app.Status.WorkflowRestartScheduledAt.Time).To(BeTemporally("~", pastTime, 1*time.Second))

			// Trigger restart - should restart because time has passed
			handler.CheckWorkflowRestart(logCtx, app)

			// Status field cleared, workflow restarted
			Expect(app.Status.WorkflowRestartScheduledAt).To(BeNil())  // Cleared after restart
			Expect(app.Status.Workflow.AppRevision).To(Equal("app-v2"))
			Expect(app.Status.Workflow.Finished).To(BeFalse())
		})

		It("Test workflow restart via annotation with future timestamp", func() {
			futureTime := time.Now().Add(1 * time.Hour)
			futureTimeStr := futureTime.Format(time.RFC3339)
			app := &oamcore.Application{
				TypeMeta: metav1.TypeMeta{Kind: "Application", APIVersion: "core.oam.dev/v1beta1"},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "app-with-future-timestamp",
					Namespace: namespaceName,
					Annotations: map[string]string{
						oam.AnnotationWorkflowRestart: futureTimeStr,
					},
				},
				Spec: oamcore.ApplicationSpec{
					Components: []common.ApplicationComponent{
						{
							Name:       "myweb",
							Type:       "worker-with-health",
							Properties: &runtime.RawExtension{Raw: []byte(`{"cmd":["sleep","1000"],"image":"busybox"}`)},
						},
					},
				},
				Status: common.AppStatus{
					Workflow: &common.WorkflowStatus{
						AppRevision: "app-v1",
						Finished:    true,
					},
					Services: []common.ApplicationComponentStatus{
						{Name: "myweb", Healthy: true},
					},
				},
			}

			handler, err := NewAppHandler(ctx, reconciler, app)
			Expect(err).Should(Succeed())

			appRev := &oamcore.ApplicationRevision{
				ObjectMeta: metav1.ObjectMeta{Name: "app-v2", Namespace: namespaceName},
			}
			handler.currentAppRev = appRev
			handler.latestAppRev = appRev

			logCtx := monitorContext.NewTraceContext(ctx, "")

			// Simulate controller processing annotation
			app.Status.WorkflowRestartScheduledAt = &metav1.Time{Time: futureTime}
			delete(app.Annotations, oam.AnnotationWorkflowRestart)

			Expect(app.Annotations).NotTo(HaveKey(oam.AnnotationWorkflowRestart))  // Annotation removed
			Expect(app.Status.WorkflowRestartScheduledAt).NotTo(BeNil())            // Status field set
			Expect(app.Status.WorkflowRestartScheduledAt.Time).To(BeTemporally("~", futureTime, 1*time.Second))

			// Trigger check - should NOT restart because time hasn't arrived
			handler.CheckWorkflowRestart(logCtx, app)

			// Workflow NOT restarted - status field still present
			Expect(app.Status.WorkflowRestartScheduledAt).NotTo(BeNil())            // Status field remains (time not arrived)
			Expect(app.Status.Workflow.AppRevision).To(Equal("app-v1"))             // Still old revision
			Expect(app.Status.Workflow.Finished).To(BeTrue())                       // Still finished
			Expect(app.Status.Services).To(HaveLen(1))                              // Services not cleared
		})

		It("Test workflow restart via annotation with duration", func() {
			// Workflow finished 2 minutes ago
			workflowEndTime := time.Now().Add(-2 * time.Minute)

			app := &oamcore.Application{
				TypeMeta: metav1.TypeMeta{Kind: "Application", APIVersion: "core.oam.dev/v1beta1"},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "app-with-duration",
					Namespace: namespaceName,
					Annotations: map[string]string{
						oam.AnnotationWorkflowRestart: "5m", // Restart 5 minutes after last completion
					},
				},
				Spec: oamcore.ApplicationSpec{
					Components: []common.ApplicationComponent{
						{
							Name:       "myweb",
							Type:       "worker-with-health",
							Properties: &runtime.RawExtension{Raw: []byte(`{"cmd":["sleep","1000"],"image":"busybox"}`)},
						},
					},
				},
				Status: common.AppStatus{
					Workflow: &common.WorkflowStatus{
						AppRevision: "app-v1",
						Finished:    true,
						EndTime:     metav1.Time{Time: workflowEndTime},
					},
					Services: []common.ApplicationComponentStatus{
						{Name: "myweb", Healthy: true},
					},
				},
			}

			handler, err := NewAppHandler(ctx, reconciler, app)
			Expect(err).Should(Succeed())

			appRev := &oamcore.ApplicationRevision{
				ObjectMeta: metav1.ObjectMeta{Name: "app-v2", Namespace: namespaceName},
			}
			handler.currentAppRev = appRev
			handler.latestAppRev = appRev

			logCtx := monitorContext.NewTraceContext(ctx, "")

			// Before annotation handling
			Expect(app.Annotations).To(HaveKey(oam.AnnotationWorkflowRestart))
			Expect(app.Status.WorkflowRestartScheduledAt).To(BeNil())

			// Simulate controller processing duration annotation
			expectedTime := workflowEndTime.Add(5 * time.Minute) // Last end + 5m = 3m from now
			app.Status.WorkflowRestartScheduledAt = &metav1.Time{Time: expectedTime}
			// For durations, annotation persists (not removed by controller)

			// For durations, annotation PERSISTS (recurring behavior), status field set
			Expect(app.Annotations).To(HaveKey(oam.AnnotationWorkflowRestart)) // Annotation KEPT for recurring
			Expect(app.Annotations[oam.AnnotationWorkflowRestart]).To(Equal("5m"))
			Expect(app.Status.WorkflowRestartScheduledAt).NotTo(BeNil())
			Expect(app.Status.WorkflowRestartScheduledAt.Time).To(BeTemporally("~", expectedTime, 1*time.Second))

			// Check workflow restart - should NOT restart yet (time not arrived)
			handler.CheckWorkflowRestart(logCtx, app)

			// Status field still present, workflow NOT restarted
			Expect(app.Status.WorkflowRestartScheduledAt).NotTo(BeNil())
			Expect(app.Status.Workflow.AppRevision).To(Equal("app-v1")) // Still old revision
			Expect(app.Status.Workflow.Finished).To(BeTrue())            // Still finished
			Expect(app.Status.Services).To(HaveLen(1))                   // Services not cleared
		})

		It("Test workflow restart with duration recurs after completion", func() {
			// Initial workflow finished 10 minutes ago
			firstEndTime := time.Now().Add(-10 * time.Minute)

			app := &oamcore.Application{
				TypeMeta: metav1.TypeMeta{Kind: "Application", APIVersion: "core.oam.dev/v1beta1"},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "app-with-recurring-duration",
					Namespace: namespaceName,
					Annotations: map[string]string{
						oam.AnnotationWorkflowRestart: "5m", // Recurring every 5m
					},
				},
				Spec: oamcore.ApplicationSpec{
					Components: []common.ApplicationComponent{
						{
							Name:       "myweb",
							Type:       "worker-with-health",
							Properties: &runtime.RawExtension{Raw: []byte(`{"cmd":["sleep","1000"],"image":"busybox"}`)},
						},
					},
				},
				Status: common.AppStatus{
					Workflow: &common.WorkflowStatus{
						AppRevision: "app-v1",
						Finished:    true,
						EndTime:     metav1.Time{Time: firstEndTime},
					},
				},
			}

			handler, err := NewAppHandler(ctx, reconciler, app)
			Expect(err).Should(Succeed())

			appRev := &oamcore.ApplicationRevision{
				ObjectMeta: metav1.ObjectMeta{Name: "app-v2", Namespace: namespaceName},
			}
			handler.currentAppRev = appRev
			handler.latestAppRev = appRev

			logCtx := monitorContext.NewTraceContext(ctx, "")

			// Simulate controller processing first scheduling: firstEndTime + 5m (5 minutes ago, ready to trigger)
			firstScheduledTime := firstEndTime.Add(5 * time.Minute)
			app.Status.WorkflowRestartScheduledAt = &metav1.Time{Time: firstScheduledTime}
			// Duration annotation persists
			Expect(app.Annotations).To(HaveKey(oam.AnnotationWorkflowRestart)) // Annotation persists
			Expect(app.Status.WorkflowRestartScheduledAt.Time).To(BeTemporally("~", firstScheduledTime, 1*time.Second))

			// Trigger restart (time has passed)
			handler.CheckWorkflowRestart(logCtx, app)
			Expect(app.Status.WorkflowRestartScheduledAt).To(BeNil()) // Cleared after restart
			Expect(app.Status.Workflow.AppRevision).To(Equal("app-v2"))

			// Simulate workflow completing again (new EndTime)
			secondEndTime := time.Now().Add(-2 * time.Minute)
			app.Status.Workflow.Finished = true
			app.Status.Workflow.EndTime = metav1.Time{Time: secondEndTime}

			// Simulate controller processing second scheduling: should recalculate based on NEW EndTime
			secondScheduledTime := secondEndTime.Add(5 * time.Minute) // 2 min ago + 5m = 3m from now
			app.Status.WorkflowRestartScheduledAt = &metav1.Time{Time: secondScheduledTime}
			Expect(app.Annotations).To(HaveKey(oam.AnnotationWorkflowRestart))             // Still persists
			Expect(app.Status.WorkflowRestartScheduledAt).NotTo(BeNil())                   // Rescheduled
			Expect(app.Status.WorkflowRestartScheduledAt.Time).To(BeTemporally("~", secondScheduledTime, 1*time.Second))

			// This time it shouldn't trigger yet (time not arrived)
			handler.CheckWorkflowRestart(logCtx, app)
			Expect(app.Status.WorkflowRestartScheduledAt).NotTo(BeNil())    // Still scheduled
			Expect(app.Status.Workflow.AppRevision).To(Equal("app-v2"))     // No change
		})

		It("Test workflow restart ignored when workflow is not finished", func() {
			pastTime := time.Now().Add(-1 * time.Hour)
			pastTimeStr := pastTime.Format(time.RFC3339)

			app := &oamcore.Application{
				TypeMeta: metav1.TypeMeta{Kind: "Application", APIVersion: "core.oam.dev/v1beta1"},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "app-with-running-workflow",
					Namespace: namespaceName,
					Annotations: map[string]string{
						oam.AnnotationWorkflowRestart: pastTimeStr,
					},
				},
				Spec: oamcore.ApplicationSpec{
					Components: []common.ApplicationComponent{
						{
							Name:       "myweb",
							Type:       "worker-with-health",
							Properties: &runtime.RawExtension{Raw: []byte(`{"cmd":["sleep","1000"],"image":"busybox"}`)},
						},
					},
				},
				Status: common.AppStatus{
					Workflow: &common.WorkflowStatus{
						AppRevision: "app-v1",
						Finished:    false, // Workflow is still running
					},
					Services: []common.ApplicationComponentStatus{
						{Name: "myweb", Healthy: true},
					},
				},
			}

			handler, err := NewAppHandler(ctx, reconciler, app)
			Expect(err).Should(Succeed())

			appRev := &oamcore.ApplicationRevision{
				ObjectMeta: metav1.ObjectMeta{Name: "app-v2", Namespace: namespaceName},
			}
			handler.currentAppRev = appRev
			handler.latestAppRev = appRev

			logCtx := monitorContext.NewTraceContext(ctx, "")

			// Simulate controller processing annotation
			app.Status.WorkflowRestartScheduledAt = &metav1.Time{Time: pastTime}
			delete(app.Annotations, oam.AnnotationWorkflowRestart)

			Expect(app.Annotations).NotTo(HaveKey(oam.AnnotationWorkflowRestart))  // Annotation removed
			Expect(app.Status.WorkflowRestartScheduledAt).NotTo(BeNil())            // Status field set

			// Check workflow restart - should be IGNORED because workflow not finished
			handler.CheckWorkflowRestart(logCtx, app)

			// Restart ignored - status field cleared but workflow NOT restarted
			Expect(app.Status.WorkflowRestartScheduledAt).To(BeNil())  // Status field cleared (consumed)
			Expect(app.Status.Workflow.AppRevision).To(Equal("app-v1"))         // Still old revision
			Expect(app.Status.Workflow.Finished).To(BeFalse())                  // Still not finished
			Expect(app.Status.Services).To(HaveLen(1))                          // Services NOT cleared
		})
	*/
})
