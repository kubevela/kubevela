import "time"

"atmos-s3-v1": {
	type: "component"
	attributes: {
		workload: definition: {
			apiVersion: "objectstore.atmos.guidewire.com/v1alpha1"
			kind:       "S3"
		}

		status: {
			// isHealth is true when the output (Crossplane claim) is ready and synced.
			// Note: For a short period after initial creation, the claim will not yet have conditions,
			// so we must do a positive test ("is ready") instead of a negative test ("isn't not ready").
			healthPolicy: #"""
				ready: bool | *false
				synced: bool | *false
				if context.output.status != _|_ {
					if context.output.status.conditions != _|_ {
						for c in context.output.status.conditions if c.type != _|_ {
							if c.type == "Ready" && c.status == "True" {
								ready: true
							}
							if c.type == "Synced" && c.status == "True" {
								synced: true
							}
						}
					}
				}
				isHealth: ready && synced
				"""#
			// message explains the meaning of isHealth.
			// Note: For single-output components, KubeVela includes the resource key
			// in the status object, so we don't have to specify it in the message.
			customStatus: #"""
				ready: bool | *false
				synced: bool | *false
				if context.output.status != _|_ {
					if context.output.status.conditions != _|_ {
						for c in context.output.status.conditions if c.type != _|_ {
							if c.type == "Ready" && c.status == "True" {
								ready: true
							}
							if c.type == "Synced" && c.status == "True" {
								synced: true
							}
						}
					}
				}
				isHealth: ready && synced

				bucketArn: "arn:aws:s3:::" + context.output.metadata.name

				if isHealth {
					message: "Bucket claim is ready/synced. bucket ARN: " + bucketArn
				}

				if !isHealth {
					message: "Bucket claim is not ready/synced."
				}
				"""#
		}
	}
}

template: {
	output: {
		apiVersion: "objectstore.atmos.guidewire.com/v1alpha1"
		kind:       "S3"
		metadata: {
			if parameter.existingResources == false {
				name: "tenant-" + parameter.governance.tenantName + "-" + parameter.name
			}
			if parameter.existingResources == true {
				name: parameter.name
			}
			namespace: context.namespace
		}
		spec: {
			if parameter.existingResources == false {
				name: "tenant-" + parameter.governance.tenantName + "-" + parameter.name
			}
			if parameter.existingResources == true {
				name: parameter.name
			}
			region: parameter.region
			tags: {
				"gwcp:v1:dept":                            parameter.governance.departmentCode
				"gwcp:v1:provisioned-resource:created-by": parameter.governance.createdBy
				"gwcp:v1:quadrant:name":                   parameter.governance.quadrantName
				"gwcp:v1:resource-type:managed-by":        "pod-ajanta"
				"gwcp:v1:resource-type:managed-tool":      "crossplane"
				"gwcp:v1:star-system:name":                parameter.governance.starSystemName
				"gwcp:v1:tenant:name":                     parameter.governance.tenantName
				"gwcp:v1:tenant:app-name":                 context.appName
			}
			if parameter.tags != _|_ {
				tags: parameter.tags
			}
			compositionRef:
				name: "s3.objectstore.atmos.guidewire.com"
			if parameter.corsRules != _|_ {
				if len(parameter.corsRules) > 0 {
					corsRules: parameter.corsRules
				}
			}
			if parameter.versioningEnabled != _|_ {
				versioningEnabled: parameter.versioningEnabled
			}
			if parameter.sseAlgorithm != _|_ {
				sseAlgorithm: parameter.sseAlgorithm
			}
			if parameter.kmsMasterKeyId != _|_ {
				kmsMasterKeyId: parameter.kmsMasterKeyId
			}
			if parameter.bucketKeyEnabled != _|_ {
				bucketKeyEnabled: parameter.bucketKeyEnabled
			}
			if parameter.forceDestroy != _|_ {
				forceDestroy: parameter.forceDestroy
			}
			if parameter.objectLock != _|_ {
				objectLock: parameter.objectLock
			}
			if parameter.replicationConfiguration != _|_ {
				replicationConfiguration: {
					if parameter.replicationConfiguration.destinationBucketRegion != _|_ {
						destinationBucketRegion: parameter.replicationConfiguration.destinationBucketRegion
					}
					if parameter.replicationConfiguration.role != _|_ {
						role: parameter.replicationConfiguration.role
					}
					if parameter.existingResources == false {
						if parameter.replicationConfiguration.destinationBucketSuffix != _|_ {
							destinationBucketName: "tenant-" + parameter.governance.tenantName + "-" + parameter.name + "-" + parameter.replicationConfiguration.destinationBucketSuffix
						}
						if parameter.replicationConfiguration.destinationBucketSuffix == _|_ {
							destinationBucketName: "tenant-" + parameter.governance.tenantName + "-" + parameter.name + "-replica"
						}
						if parameter.replicationConfiguration.role == _|_ {
							role: "atmos-s3-replication-role"
						}
					}
					if parameter.existingResources == true {
						destinationBucketName: parameter.replicationConfiguration.destinationBucketName
					}
					if parameter.replicationConfiguration.kmsKeyArn != _|_ {
						kmsKeyArn: parameter.replicationConfiguration.kmsKeyArn
					}
					if parameter.replicationConfiguration.biDirectionalReplicationEnabled != _|_ {
						biDirectionalReplicationEnabled: parameter.replicationConfiguration.biDirectionalReplicationEnabled
					}
					if parameter.replicationConfiguration.deleteMarkerReplicationEnabled != _|_ {
						deleteMarkerReplicationEnabled: parameter.replicationConfiguration.deleteMarkerReplicationEnabled
					}
					if parameter.replicationConfiguration.replicationTimeControlEnabled != _|_ {
						replicationTimeControlEnabled: parameter.replicationConfiguration.replicationTimeControlEnabled
					}
				}
			}
			if parameter.lifecycleRules != _|_ {
				if len(parameter.lifecycleRules) > 0 {
					lifecycleRules: parameter.lifecycleRules
				}
			}
			if parameter.bucketPolicy != _|_ {
				bucketPolicy: parameter.bucketPolicy
			}
			managementPolicies: parameter.managementPolicies
		}
	}

	parameter: {
		existingResources: bool | *false
		// +usage=Name of the S3 bucket. Tenant prefix will be added automatically.
		name: string & !="" & =~"^[a-z0-9.-]{3,63}$" & {
			// Validate combined length with tenant prefix
			if existingResources == false {
				if len("tenant-"+parameter.governance.tenantName+"-"+name) > 63 {
					_|_ & {
						errorMessage: """
					Combined name ("tenant-"+governance.tenantName+"-"+name) must be less than 64 characters
					"""
					}
				}
				if parameter.replicationConfiguration != _|_ {
					if parameter.replicationConfiguration.destinationBucketSuffix != _|_ {
						if len("tenant-"+parameter.governance.tenantName+"-"+name+"-"+parameter.replicationConfiguration.destinationBucketSuffix) > 63 {
							_|_ & {
								errorMessage: """
					    Combined name ("tenant-"+governance.tenantName+"-"+name+"-"+replicationConfiguration.destinationBucketSuffix) must be less than 64 characters
					    """
							}
						}
					}
					if parameter.replicationConfiguration.destinationBucketSuffix == _|_ {
						if len("tenant-"+parameter.governance.tenantName+"-"+name+"-replica") > 63 {
							_|_ & {
								errorMessage: """
					    Combined name ("tenant-"+governance.tenantName+"-"+name+"-replica") must be less than 64 characters
					    """
							}
						}
					}
				}
			}
		}

		// +usage=AWS region where the S3 bucket will be created.
		region: string & !="" & !~".*-$"

		// +usage=Object lock configuration for the bucket (optional)
		//for default values, use `objectLock: {}` in the component spec
		// +usage=If objectLock is not specified, this feature will not be enabled.
		objectLock?: {
			if existingResources == false {
				// +usage=Number of days for which the object lock will be retained
				// TODO: Handle default values.
				retentionDays?: *45 | int & {
					if retentionDays < 1 {
						_|_ & {
							errorMessage: "retentionDays must be greater than or equal to 1"
						}
					}
				}
				// +usage=Mode of retention for the object lock. Can be either "GOVERNANCE" or "COMPLIANCE"
				retentionMode?: *"GOVERNANCE" | "COMPLIANCE"
			}
			if existingResources == true {
				// +usage=Number of days for which the object lock will be retained
				// TODO: Handle defaul values.
				retentionDays: int & {
					if retentionDays < 1 {
						_|_ & {
							errorMessage: "retentionDays must be greater than or equal to 1"
						}
					}
				}
				// +usage=Mode of retention for the object lock. Can be either "GOVERNANCE" or "COMPLIANCE"
				retentionMode: "GOVERNANCE" | "COMPLIANCE"
			}
		}
		// +usage=CORS rules for the S3 bucket (optional)
		corsRules?: [...{
			// +usage=Allowed HTTP methods for the CORS rule
			allowedMethods: [...("GET" | "PUT" | "HEAD" | "POST" | "DELETE")] & {
				if len(allowedMethods) == 0 {
					_|_ & {
						errorMessage: "allowedMethods cannot be empty - at least one method is required"
					}
				}
			}
			// +usage=Allowed origins for the CORS rule
			allowedOrigins: [...string & !=""] & {
				if len(allowedOrigins) == 0 {
					_|_ & {
						errorMessage: "allowedOrigins cannot be empty - at least one origin is required"
					}
				}
			}
			// +usage=Allowed headers for the CORS rule
			allowedHeaders?: [...string & !=""]
			// +usage=Expose headers for the CORS rule
			exposeHeaders?: [...string & !=""]
			// +usage=Max age in seconds for the CORS rule
			maxAgeSeconds: *0 | int & {
				if maxAgeSeconds < 0 {
					_|_ & {
						errorMessage: "maxAgeSeconds must be greater than or equal to 0"
					}
				}
			}
		}]
		// +usage=Replication configuration for the bucket.
		replicationConfiguration?: {
			if existingResources == false {
				// +usage=IAM role ARN used by S3 for replication.
				role?: *"atmos-s3-replication-role" | (string & !="")
				// +usage=Suffix to append to the destination bucket name.
				destinationBucketSuffix?: *"replica" | (string & !="")
				// +usage=Region of the destination bucket for replication.
				destinationBucketRegion?: *"us-east-2" | (string & !="")
				// +usage=KMS key name for encrypting replicated objects.
				kmsKeyArn?: string & !=""
				// +usage=Enable bi-directional replication between source and destination buckets.
				biDirectionalReplicationEnabled?: *false | bool
				// +usage=Enable delete marker replication between source and destination buckets.
				deleteMarkerReplicationEnabled?: *false | bool
				// +usage=Enable replication time control to ensure 99.99% objects get replicated within 15 minutes.
				replicationTimeControlEnabled?: *false | bool
			}
			if existingResources == true {
				// +usage=IAM role ARN used by S3 for replication.
				role?: (string & !="")
				// +usage=Region of the destination bucket for replication.
				destinationBucketRegion: string & !=""
				// +usage=KMS key name for encrypting replicated objects.
				kmsKeyArn?:            string & !=""
				destinationBucketName: string & =~"^[a-z0-9.-]{3,63}$"
				// +usage=Enable bi-directional replication between source and destination buckets.
				biDirectionalReplicationEnabled?: *false | bool
				// +usage=Enable delete marker replication between source and destination buckets.
				deleteMarkerReplicationEnabled?: *false | bool
				// +usage=Enable replication time control to ensure 99.99% objects get replicated within 15 minutes.
				replicationTimeControlEnabled?: *false | bool
			}
		}
		// +usage=Atmos Governance metadata used for attribution of resources in Kubernetes and AWS.
		governance: close({
			// +usage=Tenant name. Will be prefixed to the requested name of the Table.
			tenantName: string & !=""

			_validateTenantName: {
				"tenantName must not end with a hyphen": true
				if tenantName != _|_ {
					if tenantName =~ ".*-$" {
						"tenantName must not end with a hyphen": false
					}
				}
			}
			// +usage=Department code, used for attributing resources to the appropriate cost center.
			departmentCode: string & !=""

			_validateDepartmentCode: {
				"departmentCode must be a numeric string": true
				"departmentCode must not start with 0":    true
				if departmentCode != _|_ {
					if !(departmentCode =~ "^[0-9]+$") {
						"departmentCode must be a numeric string": false
					}
					if departmentCode =~ "^[0-9]+$" {
						if departmentCode =~ "^0" {
							"departmentCode must not start with 0": false
						}
					}
				}
			}

			// +usage=Username of the person who is creating this resource.
			createdBy: string & !=""

			_validateCreatedBy: {
				"createdBy must not end with a hyphen": true
				if createdBy != _|_ {
					if createdBy =~ ".*-$" {
						"createdBy must not end with a hyphen": false
					}
				}
			}
			// +usage=Star system where the resource will be created.
			starSystemName: string & !=""

			_validateStarSystemName: {
				"starSystemName must not end with a hyphen": true
				if starSystemName != _|_ {
					if starSystemName =~ ".*-$" {
						"starSystemName must not end with a hyphen": false
					}
				}
			}
			// +usage=Quadrant where the resource will be created.
			quadrantName: string & !=""

			_validateQuadrantName: {
				"quadrantName must not end with a hyphen": true
				if quadrantName != _|_ {
					if quadrantName =~ ".*-$" {
						"quadrantName must not end with a hyphen": false
					}
				}
			}
		})
		// +usage=Lifecycle configuration for the bucket.
		lifecycleRules?: [...{
			// +usage=Unique identifier for the lifecycle rule.
			id?: string & !="" & {
				if len(id) > 255 {
					_|_ & {
						errorMessage: "id cannot be more than 255 characters for a lifecycle rule."
					}
				}
			}
			_validateId: {
				"lifecycleRules.id must be provided and cannot be empty": true
				if id == _|_ {
					"lifecycleRules.id must be provided and cannot be empty": false
				}
			}
			// +usage=Status of the lifecycle rule. Valid values are "Enabled", "Disabled". If not specified, status will be Enabled by default.
			status: *"Enabled" | "Disabled"
			// +usage=Abort incomplete multipart uploads after a specified number of days.
			abortIncompleteMultipartUpload?: [...{
				// +usage=Number of days after initiation to abort incomplete multipart uploads.
				daysAfterInitiation: int & {
					if daysAfterInitiation < 1 {
						_|_ & {
							errorMessage: "daysAfterInitiation under abortIncompleteMultipartUpload must be greater than 0 for a lifecycle rule."
						}
					}
				}
			}]
			// +usage=Filter for selecting objects to which the rule applies.
			filter?: [...{
				// +usage=Prefix for filtering objects.
				prefix?: string
				// +usage=Tags for filtering objects.
				tag?: [...{
					// +usage=Tag key.
					key?: string & !="" & {
						if len(key) > 128 {
							_|_ & {
								errorMessage: "maximum length for key under filter.tag is 128 characters for a lifecycle rule."
							}
						}
						if abortIncompleteMultipartUpload != _|_ {
							_|_ & {
								errorMessage: "both tag under filter and abortIncompleteMultipartUpload cannot be specified for a lifecycle rule."
							}
						}
					}
					// +usage=Tag value.
					value?: string & {
						if len(value) > 256 {
							_|_ & {
								errorMessage: "maximum length for value under filter.tag is 256 characters for a lifecycle rule."
							}
						}
					}
					_validateKey: {
						"lifecycleRules.filter.tag.key must be provided and cannot be empty": true
						if key == _|_ {
							"lifecycleRules.filter.tag.key must be provided and cannot be empty": false
						}
					}
				}]
			}]
			// +usage="Lifecycle configuration for expiring objects."
			expiration?: [...{
				// +usage=Indicates whether Amazon S3 will remove a delete marker with no noncurrent versions. If set to true, the delete marker will be expired; if set to false the policy takes no action.
				expiredObjectDeleteMarker: bool | *false
				// +usage=Number of days after object creation to expire the object.
				days?: int & {
					if expiredObjectDeleteMarker == true {
						_|_ & {
							errorMessage: "both expiredObjectDeleteMarker and days should not be specified under expiration for a lifecycle rule."
						}
					}
				}
				// +usage=Specific date to expire the object, in RFC3339 format (e.g., 2023-01-13T00:00:00Z, midnight UTC).
				date?: string & =~"^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}Z$" & {
					if expiredObjectDeleteMarker == true {
						_|_ & {
							errorMessage: "both expiredObjectDeleteMarker and date should not be specified under expiration for a lifecycle rule."
						}
					}
					if days != _|_ {
						_|_ & {
							errorMessage: "both date and days should not be specified under expiration for a lifecycle rule."
						}
					}
				}
			}] & {
				if len(expiration) > 1 {
					_|_ & {
						errorMessage: "expiration must have a length of at most 1 for a lifecycle rule."
					}
				}
			}
			// +usage=Transition rules for changing the storage class of objects.
			transition?: [...{
				// +usage=Number of days after object creation to transition to the specified storage class.
				days?: int & {
					if days < 0 {
						_|_ & {
							errorMessage: "transition days cannot be less than 0 for a lifecycle rule."
						}
					}
					if expiration != _|_ {
						if len(expiration) > 0 {
							if expiration[0].date != _|_ {
								_|_ & {
									errorMessage: "combination of expiration date and transition days is not allowed for a lifecycle rule."
								}
							}
							if expiration[0].days != _|_ {
								if days >= expiration[0].days {
									_|_ & {
										errorMessage: "expiration days must be greater than transition days for a lifecycle rule."
									}
								}
							}
						}
					}
				}
				// +usage=Specific date to transition the object, in RFC3339 format (e.g., 2023-01-13T00:00:00Z, midnight UTC).
				date?: string & =~"^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}Z$" & {
					if days != _|_ {
						_|_ & {
							errorMessage: "both days and date cannot be specified under transition for a lifecycle rule."
						}
					}
					if expiration != _|_ {
						if len(expiration) > 0 {
							if expiration[0].days != _|_ {
								_|_ & {
									errorMessage: "combination of expiration days and transition date is not allowed for a lifecycle rule."
								}
							}
							if expiration[0].date != _|_ {
								if time.Parse("2006-01-02T15:04:05Z", date) >= time.Parse("2006-01-02T15:04:05Z", expiration[0].date) {
									_|_ & {
										errorMessage: "expiration date should be later than transition date for a lifecycle rule."
									}
								}
							}
						}
					}
				}
				// +usage=Storage class to transition the object to.
				storageClass?: "STANDARD_IA" | "INTELLIGENT_TIERING" | "ONEZONE_IA" | "GLACIER" | "DEEP_ARCHIVE" | "GLACIER_IR"
				_validateStorageClass: {
					"lifecycleRules.transition.storageClass must be provided and cannot be empty": true
					if storageClass == _|_ {
						"lifecycleRules.transition.storageClass must be provided and cannot be empty": false
					}
				}
			}]
			// +usage=Lifecycle configuration for expiring noncurrent versions of objects.
			noncurrentVersionExpiration?: [...{
				// +usage=Number of days after which noncurrent versions of the object are expired.
				noncurrentDays?: int & {
					if noncurrentDays <= 0 {
						_|_ & {
							errorMessage: "noncurrentDays under noncurrentVersionExpiration must be greater than 0 for a lifecycle rule."
						}
					}
				}
			}] & {
				if len(noncurrentVersionExpiration) > 1 {
					_|_ & {
						errorMessage: "noncurrentVersionExpiration must have a length of at most 1 for a lifecycle rule."
					}
				}
			}
			// +usage=Lifecycle configuration for transitioning noncurrent versions of objects.
			noncurrentVersionTransition?: [...{
				// +usage=Number of days after which noncurrent versions of the object are transitioned.
				noncurrentDays?: int & {
					if noncurrentDays < 0 {
						_|_ & {
							errorMessage: "noncurrentDays under noncurrentVersionTransition cannot be less than 0 for a lifecycle rule."
						}
					}
					if noncurrentVersionExpiration != _|_ {
						if len(noncurrentVersionExpiration) > 0 {
							if noncurrentVersionExpiration[0].noncurrentDays != _|_ {
								if noncurrentDays >= noncurrentVersionExpiration[0].noncurrentDays {
									_|_ & {
										errorMessage: "noncurrentDays under noncurrentVersionExpiration must be greater than noncurrentDays under noncurrentVersionTransition for a lifecycle rule."
									}
								}
							}
						}
					}
				}
				// +usage=Storage class to transition the object to.
				storageClass?: "STANDARD_IA" | "INTELLIGENT_TIERING" | "ONEZONE_IA" | "GLACIER" | "DEEP_ARCHIVE" | "GLACIER_IR"
				_validateNonCurrentDays: {
					"lifecycleRules.noncurrentVersionTransition.noncurrentDays must be provided and cannot be empty": true
					if noncurrentDays == _|_ {
						"lifecycleRules.noncurrentVersionTransition.noncurrentDays must be provided and cannot be empty": false
					}
				}
				_validateNonCurrentStorageClass: {
					"lifecycleRules.noncurrentVersionTransition.storageClass must be provided and cannot be empty": true
					if storageClass == _|_ {
						"lifecycleRules.noncurrentVersionTransition.storageClass must be provided and cannot be empty": false
					}
				}
			}]
			_validateLifecycleRules: {
				"at least 1 of abortIncompleteMultipartUpload, expiration, transition, noncurrentVersionExpiration and noncurrentVersionTransition should be specified for a lifecycle rule": true
				if abortIncompleteMultipartUpload == _|_ {
					if expiration == _|_ {
						if transition == _|_ {
							if noncurrentVersionExpiration == _|_ {
								if noncurrentVersionTransition == _|_ {
									"at least 1 of abortIncompleteMultipartUpload, expiration, transition, noncurrentVersionExpiration and noncurrentVersionTransition should be specified for a lifecycle rule": false
								}
							}
						}
					}
				}
			}
		}]
		// +usage=S3 bucket policy document
		bucketPolicy?: {
			// +usage=policy language version
			Version: *"2012-10-17" | "2008-10-17"
			// +usage=List of policy statements
			Statement: [...{
				// +usage=Statement ID - must be unique within the policy
				Sid?: string
				// +usage=Whether to allow or deny the specified actions
				Effect?: "Allow" | "Deny"
				// +usage=The principal(s) allowed or denied access
				Principal?: {
					// +usage=AWS account ID, IAM user, role, or root ARN(s)
					AWS?: [...string & !=""]
					// +usage=AWS service principal(s)
					Service?: [...string & !=""]
					// +usage=S3 canonical user ID (64-char hex string)
					CanonicalUser?: [...string & !=""]
				}
				// +usage=Exception to the Principal - all principals EXCEPT these
				NotPrincipal?: {
					// +usage=AWS account ID, IAM user, role, or root ARN(s)
					AWS?: [...string & !=""]
					// +usage=AWS service principal(s)
					Service?: [...string & !=""]
					// +usage=Federated user principal (SAML, OIDC, Cognito)
					Federated?: [...string & !=""]
					// +usage=S3 canonical user ID (64-char hex string)
					CanonicalUser?: [...string & !=""]
				}
				// +usage=S3 action(s) to allow or deny
				Action?: [...string & !=""]
				// +usage=All actions EXCEPT these
				NotAction?: [...string & !=""]
				// +usage=S3 resource ARN(s) - typically the bucket and its objects
				Resource?: [...string & !=""]
				// +usage=All resources EXCEPT these
				NotResource?: [...string & !=""]
				// +usage=Optional conditions for when the statement applies
				Condition?: {...}

				_validateEffect: {
					// Validation: Effect is required
					"Effect is required in each statement": true
					if Effect == _|_ {
						"Effect is required in each statement": false
					}
				}

				_validatePrincipal: {
					// Validation: Either Principal or NotPrincipal is required
					"Either Principal or NotPrincipal is required in each statement": true
					if Principal == _|_ && NotPrincipal == _|_ {
						"Either Principal or NotPrincipal is required in each statement": false
					}

					// Validation: Principal and NotPrincipal cannot be used together
					"Principal and NotPrincipal cannot be used together in the same statement": true
					if Principal != _|_ && NotPrincipal != _|_ {
						"Principal and NotPrincipal cannot be used together in the same statement": false
					}

					// Validation: Principal sub-fields cannot be empty arrays
					if Principal != _|_ {
						"At least one type of Principal must be specified": true
						if Principal.AWS == _|_ && Principal.Service == _|_ && Principal.CanonicalUser == _|_ {
							"At least one type of Principal must be specified": false
						}
						if Principal.AWS != _|_ {
							"Principal.AWS cannot be empty - at least one AWS principal is required": true
							if len(Principal.AWS) == 0 {
								"Principal.AWS cannot be empty - at least one AWS principal is required": false
							}
						}
						if Principal.Service != _|_ {
							"Principal.Service cannot be empty - at least one service principal is required": true
							if len(Principal.Service) == 0 {
								"Principal.Service cannot be empty - at least one service principal is required": false
							}
						}
						if Principal.CanonicalUser != _|_ {
							"Principal.CanonicalUser cannot be empty - at least one canonical user is required": true
							if len(Principal.CanonicalUser) == 0 {
								"Principal.CanonicalUser cannot be empty - at least one canonical user is required": false
							}
						}
					}

					// Validation: NotPrincipal sub-fields cannot be empty arrays
					if NotPrincipal != _|_ {
						"At least one type of NotPrincipal must be specified": true
						if NotPrincipal.AWS == _|_ && NotPrincipal.Service == _|_ && NotPrincipal.Federated == _|_ && NotPrincipal.CanonicalUser == _|_ {
							"At least one type of NotPrincipal must be specified": false
						}
						if NotPrincipal.AWS != _|_ {
							"NotPrincipal.AWS cannot be empty - at least one AWS NotPrincipal is required": true
							if len(NotPrincipal.AWS) == 0 {
								"NotPrincipal.AWS cannot be empty - at least one AWS NotPrincipal is required": false
							}
						}
						if NotPrincipal.Service != _|_ {
							"NotPrincipal.Service cannot be empty - at least one service NotPrincipal is required": true
							if len(NotPrincipal.Service) == 0 {
								"NotPrincipal.Service cannot be empty - at least one service NotPrincipal is required": false
							}
						}
						if NotPrincipal.Federated != _|_ {
							"NotPrincipal.Federated cannot be empty - at least one federated NotPrincipal is required": true
							if len(NotPrincipal.Federated) == 0 {
								"NotPrincipal.Federated cannot be empty - at least one federated NotPrincipal is required": false
							}
						}
						if NotPrincipal.CanonicalUser != _|_ {
							"NotPrincipal.CanonicalUser cannot be empty - at least one CanonicalUser NotPrincipal is required": true
							if len(NotPrincipal.CanonicalUser) == 0 {
								"NotPrincipal.CanonicalUser cannot be empty - at least one CanonicalUser NotPrincipal is required": false
							}
						}
					}
				}

				_validateAction: {
					// Validation: Either Action or NotAction is required
					"Either Action or NotAction is required in each statement": true
					if Action == _|_ && NotAction == _|_ {
						"Either Action or NotAction is required in each statement": false
					}

					// Validation: Action and NotAction cannot be used together
					"Action and NotAction cannot be used together in the same statement": true
					if Action != _|_ && NotAction != _|_ {
						"Action and NotAction cannot be used together in the same statement": false
					}

					// Validation: Action and NotAction cannot be empty arrays
					if Action != _|_ {
						"Action cannot be empty - at least one action is required": true
						if len(Action) == 0 {
							"Action cannot be empty - at least one action is required": false
						}
					}
					if NotAction != _|_ {
						"NotAction cannot be empty - at least one NotAction is required": true
						if len(NotAction) == 0 {
							"NotAction cannot be empty - at least one NotAction is required": false
						}
					}
				}

				_validateResource: {
					// Validation: Either Resource or NotResource is required
					"Either Resource or NotResource is required in each statement": true
					if Resource == _|_ && NotResource == _|_ {
						"Either Resource or NotResource is required in each statement": false
					}

					// Validation: Resource and NotResource cannot be used together
					"Resource and NotResource cannot be used together in the same statement": true
					if Resource != _|_ && NotResource != _|_ {
						"Resource and NotResource cannot be used together in the same statement": false
					}

					// Validation: Resource and NotResource cannot be empty arrays
					if Resource != _|_ {
						"Resource cannot be empty - at least one Resource is required": true
						if len(Resource) == 0 {
							"Resource cannot be empty - at least one Resource is required": false
						}
					}
					if NotResource != _|_ {
						"NotResource cannot be empty - at least one NotResource is required": true
						if len(NotResource) == 0 {
							"NotResource cannot be empty - at least one NotResource is required": false
						}
					}
				}
			}] & {
				if len(Statement) == 0 {
					_|_ & {
						errorMessage: "bucketPolicy.Statement must contain at least one Statement"
					}
				}
			}
		}

		if existingResources == false {
			// +usage=boolean flag which indicates all objects (including any locked objects) should be deleted from the bucket so that the bucket can be destroyed without error.
			forceDestroy: bool | *false
			// +usage=The server side encryption algorithm which we need to apply to the bucket. Valid values are AES256, aws:kms, aws:kms:dsse
			sseAlgorithm: *"AES256" | "aws:kms" | "aws:kms:dsse"
			// +usage=AWS KMS master key ID or ARN used for the SSE-KMS encryption. This can only be used when you set the value of sseAlgorithm as "aws:kms" or "aws:kms:dsse". The default aws/s3 AWS KMS master key is used if this element is absent while the sseAlgorithm is "aws:kms" or "aws:kms:dsse".
			kmsMasterKeyId?: string & !=""
			_validateKmsMasterKeyId: {
				"kmsMasterKeyId can only be specified when sseAlgorithm is set to 'aws:kms' or 'aws:kms:dsse'": true
				if sseAlgorithm == "AES256" && kmsMasterKeyId != _|_ {
					"kmsMasterKeyId can only be specified when sseAlgorithm is set to 'aws:kms' or 'aws:kms:dsse'": false
				}
			}

			// +usage=Whether or not to use Amazon S3 Bucket Keys for SSE-KMS. This can only be used when you set the value of sseAlgorithm as "aws:kms".
			bucketKeyEnabled?: bool
			_validateBucketKeyEnabled: {
				"bucketKeyEnabled can only be specified when sseAlgorithm is set to 'aws:kms'": true
				if sseAlgorithm != "aws:kms" && bucketKeyEnabled != _|_ {
					"bucketKeyEnabled can only be specified when sseAlgorithm is set to 'aws:kms'": false
				}
			}

			// +usage=boolean flag which allows to keep multiple versions of an object in the same AWS S3 bucket
			versioningEnabled: bool | *true
			// +usage=Management policies for the S3 resource. When using existingResources, do not include 'Create' or '*'.
			managementPolicies: [...("Create" | "Delete" | "Observe" | "*" | "Update" | "LateInitialize")] | *["*"]

			if replicationConfiguration != _|_ || objectLock != _|_ {
				_validateVersioningEnabled: {
					"Require versioningEnabled to be true if objectLock or replicationConfiguration is set": true
					if versioningEnabled == false {
						"Require versioningEnabled to be true if objectLock or replicationConfiguration is set": false
					}
				}
			}
		}

		if existingResources == true {
			// +usage=boolean flag which indicates all objects (including any locked objects) should be deleted from the bucket so that the bucket can be destroyed without error.
			forceDestroy?: bool
			// +usage=The server side encryption algorithm which we need to apply to the bucket. Valid values are AES256, aws:kms, aws:kms:dsse
			sseAlgorithm?: "AES256" | "aws:kms" | "aws:kms:dsse"
			// +usage=AWS KMS master key ID or ARN used for the SSE-KMS encryption. This can only be used when you set the value of sseAlgorithm as "aws:kms" or "aws:kms:dsse". The default aws/s3 AWS KMS master key is used if this element is absent while the sseAlgorithm is "aws:kms" or "aws:kms:dsse".
			kmsMasterKeyId?: string & !=""
			_validateKmsMasterKeyId: {
				"kmsMasterKeyId can only be specified when sseAlgorithm is set to 'aws:kms' or 'aws:kms:dsse'": true
				if sseAlgorithm == "AES256" && kmsMasterKeyId != _|_ {
					"kmsMasterKeyId can only be specified when sseAlgorithm is set to 'aws:kms' or 'aws:kms:dsse'": false
				}
			}

			// +usage=Whether or not to use Amazon S3 Bucket Keys for SSE-KMS. This can only be used when you set the value of sseAlgorithm as "aws:kms".
			bucketKeyEnabled?: bool
			_validateBucketKeyEnabled: {
				"bucketKeyEnabled can only be specified when sseAlgorithm is set to 'aws:kms'": true
				if sseAlgorithm != "aws:kms" && bucketKeyEnabled != _|_ {
					"bucketKeyEnabled can only be specified when sseAlgorithm is set to 'aws:kms'": false
				}
			}

			// +usage=boolean flag which allows to keep multiple versions of an object in the same AWS S3 bucket
			versioningEnabled?: bool
			// +usage=Management policies for the S3 resource. When using existingResources, do not include 'Create' or '*'.
			managementPolicies: [...("Create" | "Delete" | "Observe" | "*" | "Update" | "LateInitialize")] | *["Observe"]

			if replicationConfiguration != _|_ || objectLock != _|_ {
				_validateVersioningEnabled: {
					"Require versioningEnabled to be true if objectLock or replicationConfiguration is set": true
					if versioningEnabled == _|_ {
						"Require versioningEnabled to be true if objectLock or replicationConfiguration is set": false
					}
					if versioningEnabled != _|_ {
						if versioningEnabled == false {
							"Require versioningEnabled to be true if objectLock or replicationConfiguration is set": false
						}
					}
				}
			}
		}
	}
}
