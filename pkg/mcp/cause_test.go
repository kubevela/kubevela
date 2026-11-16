/*
Copyright 2026 The KubeVela Authors.

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

package mcp

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("classifyCause", func() {
	ack := func(conds ...ResourceCondition) *ResourceDiagnosis {
		return &ResourceDiagnosis{Provider: providerACK, Kind: "DBCluster", Name: "orders-db", Conditions: conds}
	}

	DescribeTable("classifies from the provider's own code token",
		func(diag *ResourceDiagnosis, class, code, owner, evidenceHas string) {
			c := classifyCause(diag)
			Expect(c).NotTo(BeNil())
			Expect(c.Class).To(Equal(class))
			Expect(c.Code).To(Equal(code))
			Expect(c.Owner).To(Equal(owner))
			Expect(c.Scope).To(Equal(scopeThisResource))
			Expect(c.Evidence).To(ContainSubstring(evidenceHas))
		},
		// ACK, live strings from the demo cluster and AWS's documented throttle form.
		Entry("ACK throttle in Recoverable", ack(
			ResourceCondition{Type: "ACK.ResourceSynced", Status: "Unknown", Reason: "operation error RDS: ModifyDBCluster, https response error StatusCode: 400, RequestID: 1, Throttling: Rate exceeded"},
			ResourceCondition{Type: "ACK.Recoverable", Status: "True", Message: "Throttling: Rate exceeded"},
		), causeThrottled, "Throttling", ownerExternal, "ACK.Recoverable"),
		Entry("ACK unresolved reference names the class by the family's own convention, ahead of a stale Synced", ack(
			ResourceCondition{Type: "ACK.ResourceSynced", Status: "True"},
			ResourceCondition{Type: "ACK.ReferencesResolved", Status: "False", Message: "resource reference wrapper or ID required: VPCID"},
		), causeDependencyMissing, "ACK.ReferencesResolved", ownerApplicationSpec, "ACK.ReferencesResolved message"),
		Entry("Google RateLimitExceeded is a throttle, not an exhausted quota", &ResourceDiagnosis{Provider: providerCrossplane, Kind: "Instance", Name: "db", Conditions: []ResourceCondition{
			{Type: "Synced", Status: "False", Reason: "ReconcileError", Message: "observe failed: googleapi: Error 429: RateLimitExceeded: Rate limit exceeded for this project"},
		}}, causeThrottled, "RateLimitExceeded", ownerExternal, "Synced"),
		Entry("ACK terminal invalid parameter", ack(
			ResourceCondition{Type: "ACK.ResourceSynced", Status: "False", Reason: "resource is in terminal condition", Message: "Resource not synced"},
			ResourceCondition{Type: "ACK.Terminal", Status: "True", Message: "api error InvalidParameterCombination: Cannot find version 16.3 for aurora-postgresql"},
		), causeInvalid, "InvalidParameterCombination", ownerApplicationSpec, "ACK.Terminal"),
		Entry("ACK InvalidSubnet is invalid, not dependency-missing", ack(
			ResourceCondition{Type: "ACK.Terminal", Status: "True", Message: "InvalidSubnet: 'subnet-does-not-exist-1' is not an EC2 subnet id (expected subnet- followed by 8 or 17 hexadecimal characters)"},
		), causeInvalid, "InvalidSubnet", ownerApplicationSpec, "ACK.Terminal"),
		Entry("ACK access denied", ack(
			ResourceCondition{Type: "ACK.Recoverable", Status: "True", Message: "AccessDeniedException: User: arn:aws:sts::1:assumed-role/x is not authorized to perform: dynamodb:CreateTable"},
		), causeAuth, "AccessDeniedException", ownerPlatform, "ACK.Recoverable"),
		Entry("ACK limit exceeded", ack(
			ResourceCondition{Type: "ACK.Recoverable", Status: "True", Message: "LimitExceededException: Subscriber limit exceeded: Only 2500 tables can be created"},
		), causeQuota, "LimitExceededException", ownerExternal, "ACK.Recoverable"),
		Entry("ACK already exists", ack(
			ResourceCondition{Type: "ACK.Terminal", Status: "True", Message: "EntityAlreadyExists: Role with name demo already exists."},
		), causeConflict, "EntityAlreadyExists", ownerApplicationSpec, "ACK.Terminal"),
		Entry("ACK endpoint unreachable", ack(
			ResourceCondition{Type: "ACK.ResourceSynced", Status: "Unknown", Reason: "operation error RDS: DescribeDBClusters, exceeded maximum number of attempts, 3, https response error StatusCode: 0, RequestID: , request send failed, Post \"http://aws-emulator.floci.svc.cluster.local:4566/\": dial tcp 10.43.0.9:4566: connect: connection refused"},
		), causeUnreachable, "connection refused", ownerPlatform, "ACK.ResourceSynced"),
		// Config Connector, message formats from its troubleshooting documentation.
		Entry("Config Connector 403", &ResourceDiagnosis{Provider: providerConfigConnector, Kind: "SpannerInstance", Name: "db", Conditions: []ResourceCondition{
			{Type: "Ready", Status: "False", Reason: "UpdateFailed", Message: "Update call failed: error fetching live state: error reading underlying resource: googleapi: Error 403: Caller is missing IAM permission spanner.instances.get on resource"},
		}}, causeAuth, "googleapi: Error 403", ownerPlatform, "Ready"),
		Entry("Config Connector dependency not found", &ResourceDiagnosis{Provider: providerConfigConnector, Kind: "SQLDatabase", Name: "db", Conditions: []ResourceCondition{
			{Type: "Ready", Status: "False", Reason: "DependencyNotFound", Message: "reference SQLInstance default/missing is not found"},
		}}, causeDependencyMissing, "DependencyNotFound", ownerApplicationSpec, "Ready"),
		Entry("Config Connector 429 rate limit is a throttle", &ResourceDiagnosis{Provider: providerConfigConnector, Kind: "SQLInstance", Name: "db", Conditions: []ResourceCondition{
			{Type: "Ready", Status: "False", Reason: "UpdateFailed", Message: "Update call failed: googleapi: Error 429: Quota exceeded for quota metric 'Queries' and limit 'Queries per minute', rateLimitExceeded"},
		}}, causeThrottled, "rateLimitExceeded", ownerExternal, "Ready"),
		Entry("Config Connector hard quota on 403 is quota-exhausted, not auth", &ResourceDiagnosis{Provider: providerConfigConnector, Kind: "SQLInstance", Name: "db", Conditions: []ResourceCondition{
			{Type: "Ready", Status: "False", Reason: "UpdateFailed", Message: "Update call failed: googleapi: Error 403: Quota exceeded for quota metric 'Instances' and limit 'Instances per project per region', quotaExceeded"},
		}}, causeQuota, "Quota exceeded", ownerExternal, "Ready"),
		// Crossplane, wrapped as crossplane-runtime's managed reconciler wraps provider errors.
		Entry("Crossplane throttle inside observe failed", &ResourceDiagnosis{Provider: providerCrossplane, Kind: "Bucket", Name: "b", Conditions: []ResourceCondition{
			{Type: "Synced", Status: "False", Reason: "ReconcileError", Message: "observe failed: cannot run refresh: refresh failed: operation error S3: HeadBucket, https response error StatusCode: 503, RequestID: x, api error SlowDown: Please reduce your request rate."},
		}}, causeThrottled, "SlowDown", ownerExternal, "Synced"),
		Entry("Crossplane expired token", &ResourceDiagnosis{Provider: providerCrossplane, Kind: "Bucket", Name: "b", Conditions: []ResourceCondition{
			{Type: "Synced", Status: "False", Reason: "ReconcileError", Message: "connect failed: cannot get AWS config: ExpiredTokenException: The security token included in the request is expired"},
		}}, causeAuth, "ExpiredTokenException", ownerPlatform, "Synced"),
		Entry("ACK throttle as the live controller writes it", ack(
			ResourceCondition{Type: "ACK.Recoverable", Status: "True", Message: "api error Throttling: Rate exceeded"},
		), causeThrottled, "Throttling", ownerExternal, "ACK.Recoverable"),
		Entry("EC2 dotted code", ack(
			ResourceCondition{Type: "ACK.Terminal", Status: "True", Message: "InvalidSubnetID.NotFound: The subnet ID 'subnet-0abc' does not exist"},
		), causeDependencyMissing, "InvalidSubnetID.NotFound", ownerApplicationSpec, "ACK.Terminal"),
		Entry("code position beats an identifier echoed in the prose", ack(
			ResourceCondition{Type: "ACK.Recoverable", Status: "True", Message: "ResourceConflictException: function OrderNotFoundHandler is updating"},
		), causeConflict, "ResourceConflictException", ownerApplicationSpec, "ACK.Recoverable"),
		Entry("SDK-wrapped code after api error is read at the code position, on a 400", ack(
			ResourceCondition{Type: "ACK.ResourceSynced", Status: "Unknown", Reason: "operation error EC2: RunInstances, https response error StatusCode: 400, RequestID: x, api error VcpuLimitExceeded: You have requested more vCPU capacity"},
		), causeQuota, "VcpuLimitExceeded", ownerExternal, "ACK.ResourceSynced"),
		Entry("codes beat phrases", ack(
			ResourceCondition{Type: "ACK.Recoverable", Status: "True", Message: "AccessDeniedException: User arn:aws:iam::1:user/x is not authorized; dial tcp 10.0.0.1:443: connect: connection refused"},
		), causeAuth, "AccessDeniedException", ownerPlatform, "ACK.Recoverable"),
		Entry("SDK-wrapped conflict beats an identifier echoed in the prose", ack(
			ResourceCondition{Type: "ACK.ResourceSynced", Status: "Unknown", Reason: "operation error Lambda: UpdateFunctionCode, https response error StatusCode: 409, RequestID: x, api error ResourceConflictException: function OrderNotFoundHandler is updating"},
		), causeConflict, "ResourceConflictException", ownerApplicationSpec, "ACK.ResourceSynced"),
		Entry("Crossplane SDK-wrapped not found", &ResourceDiagnosis{Provider: providerCrossplane, Kind: "Table", Name: "t", Conditions: []ResourceCondition{
			{Type: "Synced", Status: "False", Reason: "ReconcileError", Message: "observe failed: operation error DynamoDB: DescribeTable, https response error StatusCode: 400, RequestID: x, api error ResourceNotFoundException: Requested resource not found"},
		}}, causeDependencyMissing, "ResourceNotFoundException", ownerApplicationSpec, "Synced"),
		// A lower-camel code the token steps cannot read is matched as a phrase.
		Entry("ACME rateLimited written in lower camel case is a throttle", &ResourceDiagnosis{Provider: providerConfigConnector, Kind: "Certificate", Name: "tls", Conditions: []ResourceCondition{
			{Type: "Ready", Status: "False", Reason: "Failed", Message: "The certificate request has failed to complete and will be retried: Failed to wait for order resource \"tls-1\" to become ready: order is in \"errored\" state: Failed to create Order: 429 urn:ietf:params:acme:error:rateLimited: too many certificates (5) already issued for this exact set of domains in the last 168 hours"},
		}}, causeThrottled, "rateLimited", ownerExternal, "Ready"),
	)

	DescribeTable("leaves text no rule recognises unclassified, with no owner and no code",
		func(diag *ResourceDiagnosis) {
			c := classifyCause(diag)
			Expect(c).NotTo(BeNil())
			Expect(c.Class).To(Equal(causeUnclassified))
			Expect(c.Owner).To(BeEmpty())
			Expect(c.Code).To(BeEmpty())
		},
		Entry("sentence-initial prose", &ResourceDiagnosis{Provider: providerCrossplane, Kind: "Bucket", Name: "b", Conditions: []ResourceCondition{
			{Type: "Synced", Status: "False", Reason: "ReconcileError", Message: "connect failed: Invalid provider config"},
		}}),
		Entry("unknown AWS code", ack(
			ResourceCondition{Type: "ACK.Recoverable", Status: "True", Message: "api error PriorRequestNotComplete: Request is still in progress"},
		)),
		Entry("explicit code no rule names, with a rule token echoed in its prose", ack(
			ResourceCondition{Type: "ACK.Recoverable", Status: "True", Message: "api error DBSubnetGroupDoesNotCoverEnoughAZs: The DB subnet group does not meet Availability Zone coverage; compare with InvalidSubnet in the service reference"},
		)),
		Entry("ACK KMS key not accessible: a missing key or a denying key policy, owned by different layers", ack(
			ResourceCondition{Type: "ACK.Recoverable", Status: "True", Message: "KMSKeyNotAccessibleFault: The specified KMS key [arn:aws:kms:us-east-1:000000000000:key/7323017e] does not exist, is not enabled or you do not have permissions to access it."},
		)),
		Entry("insufficient capacity is not an exhausted quota", ack(
			ResourceCondition{Type: "ACK.Recoverable", Status: "True", Message: "api error InsufficientInstanceCapacity: We currently do not have sufficient m5.large capacity in the Availability Zone you requested"},
		)),
	)

	Describe("standard-conditions sources", func() {
		std := func(conds ...ResourceCondition) *ResourceDiagnosis {
			return &ResourceDiagnosis{Provider: providerStandardConditions, Kind: "Gateway", Name: "edge", Conditions: conds}
		}

		It("reads the Ready reason, then its message, while Ready is not True", func() {
			c := classifyCause(std(
				ResourceCondition{Type: "Accepted", Status: "False", Reason: "Invalid", Message: "AccessDenied: listener rejected"},
				ResourceCondition{Type: "Ready", Status: "False", Reason: "BackendNotFound", Message: "Service team-a/orders not found"},
			))
			Expect(c.Class).To(Equal(causeDependencyMissing))
			Expect(c.Code).To(Equal("BackendNotFound"))
			Expect(c.Evidence).To(HavePrefix("Ready reason: BackendNotFound"))
		})

		It("falls back to the newest False condition with a message when Ready is absent", func() {
			older := time.Date(2026, 11, 16, 8, 0, 0, 0, time.UTC)
			newer := time.Date(2026, 11, 16, 9, 0, 0, 0, time.UTC)
			c := classifyCause(std(
				ResourceCondition{Type: "Accepted", Status: "True"},
				ResourceCondition{Type: "ResolvedRefs", Status: "False", Reason: "RefNotPermitted", Message: "AccessDenied: cross-namespace reference", LastTransition: &older},
				ResourceCondition{Type: "Programmed", Status: "False", Reason: "Pending", Message: "dial tcp 10.0.0.1:443: connection refused", LastTransition: &newer},
				ResourceCondition{Type: "Stale", Status: "False", Reason: "Silent"},
			))
			Expect(c.Class).To(Equal(causeUnreachable))
			Expect(c.Evidence).To(HavePrefix("Programmed message: dial tcp"))
			Expect(*c.Since).To(Equal(newer))
		})

		It("falls back to the newest False condition when Ready is not True but carries no text", func() {
			c := classifyCause(std(
				ResourceCondition{Type: "Ready", Status: "False"},
				ResourceCondition{Type: "Synced", Status: "False", Reason: "QuotaExceeded", Message: "zone record quota exceeded"},
			))
			Expect(c.Class).To(Equal(causeQuota))
			Expect(c.Code).To(Equal("QuotaExceeded"))
		})

		It("returns nil for Ready True, a lone True condition, or False conditions without text", func() {
			Expect(classifyCause(&ResourceDiagnosis{Provider: providerStandardConditions, Ready: true, Conditions: []ResourceCondition{{Type: "Ready", Status: "True"}}})).To(BeNil())
			Expect(classifyCause(std(ResourceCondition{Type: "Accepted", Status: "True"}))).To(BeNil())
			Expect(classifyCause(std(ResourceCondition{Type: "Programmed", Status: "False", Reason: "Pending"}))).To(BeNil())
		})
	})

	It("ignores error fields whose condition is not true", func() {
		Expect(classifyCause(ack(
			ResourceCondition{Type: "ACK.Recoverable", Status: "False", Message: "stale"},
			ResourceCondition{Type: "ACK.Terminal", Status: "False", Message: "stale"},
			ResourceCondition{Type: "ACK.ResourceSynced", Status: "True"},
		))).To(BeNil())
	})

	It("reads Terminal before Recoverable", func() {
		c := classifyCause(ack(
			ResourceCondition{Type: "ACK.Recoverable", Status: "True", Message: "Throttling: Rate exceeded"},
			ResourceCondition{Type: "ACK.Terminal", Status: "True", Message: "AccessDeniedException: x"},
		))
		Expect(c.Class).To(Equal(causeAuth))
	})

	It("returns unclassified with the raw text and no owner when nothing matches", func() {
		c := classifyCause(ack(ResourceCondition{Type: "ACK.Recoverable", Status: "True", Message: "something the controller made up"}))
		Expect(c).NotTo(BeNil())
		Expect(c.Class).To(Equal(causeUnclassified))
		Expect(c.Owner).To(BeEmpty())
		Expect(c.Code).To(BeEmpty())
		Expect(c.Evidence).To(ContainSubstring("something the controller made up"))
	})

	It("returns nil when the resource is ready or carries no error text", func() {
		Expect(classifyCause(&ResourceDiagnosis{Provider: providerACK, Ready: true, Conditions: []ResourceCondition{{Type: "ACK.ResourceSynced", Status: "True"}}})).To(BeNil())
		Expect(classifyCause(ack(ResourceCondition{Type: "ACK.ResourceSynced", Status: "False"}))).To(BeNil())
		Expect(classifyCause(&ResourceDiagnosis{Provider: providerKubernetes})).To(BeNil())
	})

	It("ignores lower-case prose words", func() {
		// "notfound" inside a lower-case sentence is prose, not a code token.
		c := classifyCause(ack(ResourceCondition{Type: "ACK.Recoverable", Status: "True", Message: "the controller found nothing to do; notfound is just a word here"}))
		Expect(c.Class).To(Equal(causeUnclassified))
	})

	It("does not panic on non-ASCII condition text and still finds the phrase", func() {
		c := classifyCause(ack(ResourceCondition{Type: "ACK.ResourceSynced", Status: "Unknown", Reason: "İstanbul endpoint: dial tcp 10.0.0.1:4566: connect: connection refused"}))
		Expect(c.Class).To(Equal(causeUnreachable))
		Expect(c.Code).To(Equal("connection refused"))
	})

	It("treats a bare 429 as a throttle and lets the quota phrase override it", func() {
		bare := &ResourceDiagnosis{Provider: providerConfigConnector, Conditions: []ResourceCondition{{Type: "Ready", Status: "False", Reason: "UpdateFailed", Message: "Update call failed: googleapi: Error 429: too many requests"}}}
		Expect(classifyCause(bare).Class).To(Equal(causeThrottled))
		quota := &ResourceDiagnosis{Provider: providerConfigConnector, Conditions: []ResourceCondition{{Type: "Ready", Status: "False", Reason: "UpdateFailed", Message: "Update call failed: googleapi: Error 429: Quota exceeded for quota metric 'Instances', quotaExceeded"}}}
		Expect(classifyCause(quota).Class).To(Equal(causeQuota))
	})

	It("records the matched condition's transition time", func() {
		ts := time.Date(2026, 11, 15, 9, 12, 0, 0, time.UTC)
		c := classifyCause(ack(ResourceCondition{Type: "ACK.Recoverable", Status: "True", Message: "Throttling: Rate exceeded", LastTransition: &ts}))
		Expect(c.Since).NotTo(BeNil())
		Expect(*c.Since).To(Equal(ts))
	})

	It("gives every rule one known owner and reserves controller for the sweep", func() {
		known := map[string]bool{ownerApplicationSpec: true, ownerController: true, ownerExternal: true, ownerPlatform: true}
		for _, r := range classRules {
			Expect(known).To(HaveKey(r.owner), r.class)
			Expect(r.owner).NotTo(Equal(ownerController), r.class+": no rule attributes a cause to the controller's own behaviour in this release")
		}
	})

	It("names the account-level classes", func() {
		for _, cl := range []string{causeThrottled, causeQuota, causeAuth, causeUnreachable} {
			Expect(accountLevelClass(cl)).To(BeTrue(), cl)
		}
		for _, cl := range []string{causeInvalid, causeDependencyMissing, causeConflict, causeUnclassified} {
			Expect(accountLevelClass(cl)).To(BeFalse(), cl)
		}
	})
})
