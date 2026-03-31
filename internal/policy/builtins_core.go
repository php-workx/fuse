package policy

import (
	"regexp"
	"strings"

	"github.com/php-workx/fuse/internal/core"
)

func init() {
	BuiltinRules = append(BuiltinRules,

		// ===================================================================
		// §6.3.1 Git operations
		// ===================================================================
		BuiltinRule{
			ID:      "builtin:git:reset-hard",
			Pattern: regexp.MustCompile(`\bgit\s+reset\s+--hard\b`),
			Action:  core.DecisionCaution,
			Reason:  "Discards all uncommitted changes",
		},
		BuiltinRule{
			ID:      "builtin:git:clean",
			Pattern: regexp.MustCompile(`\bgit\s+clean\s+-[a-zA-Z]*f`),
			Action:  core.DecisionCaution,
			Reason:  "Deletes untracked files",
		},
		BuiltinRule{
			ID:      "builtin:git:push-force",
			Pattern: regexp.MustCompile(`\bgit\s+push\s+.*--force\b`),
			Action:  core.DecisionCaution,
			Reason:  "Force push can overwrite remote history",
		},
		BuiltinRule{
			ID:      "builtin:git:push-force-lease",
			Pattern: regexp.MustCompile(`\bgit\s+push\s+.*--force-with-lease\b`),
			Action:  core.DecisionCaution,
			Reason:  "Force push with lease",
		},
		BuiltinRule{
			ID:      "builtin:git:stash-clear",
			Pattern: regexp.MustCompile(`\bgit\s+stash\s+clear\b`),
			Action:  core.DecisionCaution,
			Reason:  "Deletes all stashed changes",
		},
		BuiltinRule{
			ID:      "builtin:git:stash-drop",
			Pattern: regexp.MustCompile(`\bgit\s+stash\s+drop\b`),
			Action:  core.DecisionCaution,
			Reason:  "Deletes a stash entry",
		},
		BuiltinRule{
			ID:      "builtin:git:branch-D",
			Pattern: regexp.MustCompile(`\bgit\s+branch\s+-D\b`),
			Action:  core.DecisionCaution,
			Reason:  "Force-deletes a branch",
		},
		BuiltinRule{
			ID:      "builtin:git:checkout-dot",
			Pattern: regexp.MustCompile(`\bgit\s+checkout\s+--\s*\.`),
			Action:  core.DecisionCaution,
			Reason:  "Discards all working tree changes",
		},
		BuiltinRule{
			ID:      "builtin:git:restore-worktree",
			Pattern: regexp.MustCompile(`\bgit\s+restore\b`),
			Action:  core.DecisionCaution,
			Reason:  "May discard working tree changes",
			Predicate: func(cmd string) bool {
				return !strings.Contains(cmd, "--staged")
			},
		},

		// ===================================================================
		// §6.3.2 Cloud: AWS — Compute & containers
		// ===================================================================
		BuiltinRule{
			ID:      "builtin:aws:terminate-instances",
			Pattern: regexp.MustCompile(`\baws\s+ec2\s+terminate-instances\b`),
			Action:  core.DecisionCaution,
			Reason:  "Terminates EC2 instances",
		},
		BuiltinRule{
			ID:      "builtin:aws:stop-instances",
			Pattern: regexp.MustCompile(`\baws\s+ec2\s+stop-instances\b`),
			Action:  core.DecisionCaution,
			Reason:  "Stops EC2 instances",
		},
		BuiltinRule{
			ID:      "builtin:aws:delete-snapshot",
			Pattern: regexp.MustCompile(`\baws\s+ec2\s+delete-snapshot\b`),
			Action:  core.DecisionCaution,
			Reason:  "Deletes EC2 snapshot",
		},
		BuiltinRule{
			ID:      "builtin:aws:delete-volume",
			Pattern: regexp.MustCompile(`\baws\s+ec2\s+delete-volume\b`),
			Action:  core.DecisionCaution,
			Reason:  "Deletes EBS volume",
		},
		BuiltinRule{
			ID:      "builtin:aws:delete-vpc",
			Pattern: regexp.MustCompile(`\baws\s+ec2\s+delete-vpc\b`),
			Action:  core.DecisionCaution,
			Reason:  "Deletes VPC",
		},
		BuiltinRule{
			ID:      "builtin:aws:delete-subnet",
			Pattern: regexp.MustCompile(`\baws\s+ec2\s+delete-subnet\b`),
			Action:  core.DecisionCaution,
			Reason:  "Deletes subnet",
		},
		BuiltinRule{
			ID:      "builtin:aws:delete-sg",
			Pattern: regexp.MustCompile(`\baws\s+ec2\s+delete-security-group\b`),
			Action:  core.DecisionCaution,
			Reason:  "Deletes security group",
		},
		BuiltinRule{
			ID:      "builtin:aws:delete-keypair",
			Pattern: regexp.MustCompile(`\baws\s+ec2\s+delete-key-pair\b`),
			Action:  core.DecisionCaution,
			Reason:  "Deletes EC2 key pair",
		},
		BuiltinRule{
			ID:      "builtin:aws:deregister-ami",
			Pattern: regexp.MustCompile(`\baws\s+ec2\s+deregister-image\b`),
			Action:  core.DecisionCaution,
			Reason:  "Deregisters AMI",
		},
		BuiltinRule{
			ID:      "builtin:aws:modify-sg-ingress",
			Pattern: regexp.MustCompile(`\baws\s+ec2\s+authorize-security-group-ingress\b`),
			Action:  core.DecisionCaution,
			Reason:  "Modifies security group ingress rules",
		},
		BuiltinRule{
			ID:      "builtin:aws:delete-ecs-service",
			Pattern: regexp.MustCompile(`\baws\s+ecs\s+delete-service\b`),
			Action:  core.DecisionCaution,
			Reason:  "Deletes ECS service",
		},
		BuiltinRule{
			ID:      "builtin:aws:delete-ecs-cluster",
			Pattern: regexp.MustCompile(`\baws\s+ecs\s+delete-cluster\b`),
			Action:  core.DecisionCaution,
			Reason:  "Deletes ECS cluster",
		},
		BuiltinRule{
			ID:      "builtin:aws:deregister-taskdef",
			Pattern: regexp.MustCompile(`\baws\s+ecs\s+deregister-task-definition\b`),
			Action:  core.DecisionCaution,
			Reason:  "Deregisters ECS task definition",
		},
		BuiltinRule{
			ID:      "builtin:aws:delete-eks-cluster",
			Pattern: regexp.MustCompile(`\baws\s+eks\s+delete-cluster\b`),
			Action:  core.DecisionCaution,
			Reason:  "Deletes EKS cluster",
		},
		BuiltinRule{
			ID:      "builtin:aws:delete-eks-nodegroup",
			Pattern: regexp.MustCompile(`\baws\s+eks\s+delete-nodegroup\b`),
			Action:  core.DecisionCaution,
			Reason:  "Deletes EKS node group",
		},

		// §6.3.2 AWS — Storage
		BuiltinRule{
			ID:      "builtin:aws:delete-bucket",
			Pattern: regexp.MustCompile(`\baws\s+s3\s+rb\b|aws\s+s3api\s+delete-bucket\b`),
			Action:  core.DecisionCaution,
			Reason:  "Deletes S3 bucket",
		},
		BuiltinRule{
			ID:      "builtin:aws:s3-rm",
			Pattern: regexp.MustCompile(`\baws\s+s3\s+rm\s+.*--recursive\b`),
			Action:  core.DecisionCaution,
			Reason:  "Recursively deletes S3 objects",
		},
		BuiltinRule{
			ID:      "builtin:aws:delete-ecr-repo",
			Pattern: regexp.MustCompile(`\baws\s+ecr\s+delete-repository\b`),
			Action:  core.DecisionCaution,
			Reason:  "Deletes ECR repository",
		},
		BuiltinRule{
			ID:      "builtin:aws:ecr-batch-delete",
			Pattern: regexp.MustCompile(`\baws\s+ecr\s+batch-delete-image\b`),
			Action:  core.DecisionCaution,
			Reason:  "Batch-deletes ECR images",
		},

		// §6.3.2 AWS — Databases & data
		BuiltinRule{
			ID:      "builtin:aws:delete-db",
			Pattern: regexp.MustCompile(`\baws\s+rds\s+delete-db-(instance|cluster)\b`),
			Action:  core.DecisionCaution,
			Reason:  "Deletes RDS database",
		},
		BuiltinRule{
			ID:      "builtin:aws:delete-table",
			Pattern: regexp.MustCompile(`\baws\s+dynamodb\s+delete-table\b`),
			Action:  core.DecisionCaution,
			Reason:  "Deletes DynamoDB table",
		},
		BuiltinRule{
			ID:      "builtin:aws:delete-elasticache",
			Pattern: regexp.MustCompile(`\baws\s+elasticache\s+delete-(cache-cluster|replication-group)\b`),
			Action:  core.DecisionCaution,
			Reason:  "Deletes ElastiCache cluster",
		},
		BuiltinRule{
			ID:      "builtin:aws:delete-kinesis",
			Pattern: regexp.MustCompile(`\baws\s+kinesis\s+delete-stream\b`),
			Action:  core.DecisionCaution,
			Reason:  "Deletes Kinesis stream",
		},

		// §6.3.2 AWS — Serverless & application
		BuiltinRule{
			ID:      "builtin:aws:delete-function",
			Pattern: regexp.MustCompile(`\baws\s+lambda\s+delete-function\b`),
			Action:  core.DecisionCaution,
			Reason:  "Deletes Lambda function",
		},
		BuiltinRule{
			ID:      "builtin:aws:delete-rest-api",
			Pattern: regexp.MustCompile(`\baws\s+apigateway\s+delete-rest-api\b`),
			Action:  core.DecisionCaution,
			Reason:  "Deletes API Gateway REST API",
		},
		BuiltinRule{
			ID:      "builtin:aws:delete-apigw-v2",
			Pattern: regexp.MustCompile(`\baws\s+apigatewayv2\s+delete-api\b`),
			Action:  core.DecisionCaution,
			Reason:  "Deletes API Gateway v2 API",
		},
		BuiltinRule{
			ID:      "builtin:aws:delete-sfn",
			Pattern: regexp.MustCompile(`\baws\s+stepfunctions\s+delete-state-machine\b`),
			Action:  core.DecisionCaution,
			Reason:  "Deletes Step Functions state machine",
		},
		BuiltinRule{
			ID:      "builtin:aws:delete-eventbridge",
			Pattern: regexp.MustCompile(`\baws\s+events\s+delete-rule\b`),
			Action:  core.DecisionCaution,
			Reason:  "Deletes EventBridge rule",
		},
		BuiltinRule{
			ID:      "builtin:aws:delete-sqs",
			Pattern: regexp.MustCompile(`\baws\s+sqs\s+delete-queue\b`),
			Action:  core.DecisionCaution,
			Reason:  "Deletes SQS queue",
		},
		BuiltinRule{
			ID:      "builtin:aws:purge-sqs",
			Pattern: regexp.MustCompile(`\baws\s+sqs\s+purge-queue\b`),
			Action:  core.DecisionCaution,
			Reason:  "Purges all messages from SQS queue",
		},
		BuiltinRule{
			ID:      "builtin:aws:delete-sns",
			Pattern: regexp.MustCompile(`\baws\s+sns\s+delete-topic\b`),
			Action:  core.DecisionCaution,
			Reason:  "Deletes SNS topic",
		},
		BuiltinRule{
			ID:      "builtin:aws:delete-ses-identity",
			Pattern: regexp.MustCompile(`\baws\s+ses\s+delete-identity\b`),
			Action:  core.DecisionCaution,
			Reason:  "Deletes SES identity",
		},

		// §6.3.2 AWS — Infrastructure & networking
		BuiltinRule{
			ID:      "builtin:aws:delete-stack",
			Pattern: regexp.MustCompile(`\baws\s+cloudformation\s+delete-stack(?:\s|$)`),
			Action:  core.DecisionCaution,
			Reason:  "Deletes CloudFormation stack",
		},
		BuiltinRule{
			ID:      "builtin:aws:delete-stack-set",
			Pattern: regexp.MustCompile(`\baws\s+cloudformation\s+delete-stack-set\b`),
			Action:  core.DecisionCaution,
			Reason:  "Deletes CloudFormation stack set (multi-account/region)",
		},
		BuiltinRule{
			ID:      "builtin:aws:delete-stack-instances",
			Pattern: regexp.MustCompile(`\baws\s+cloudformation\s+delete-stack-instances\b.*--no-retain-stacks\b`),
			Action:  core.DecisionCaution,
			Reason:  "Deletes stack set instances and their underlying stacks",
		},
		BuiltinRule{
			ID:      "builtin:aws:delete-change-set",
			Pattern: regexp.MustCompile(`\baws\s+cloudformation\s+delete-change-set\b`),
			Action:  core.DecisionCaution,
			Reason:  "Deletes CloudFormation change set",
		},
		BuiltinRule{
			ID:      "builtin:aws:cancel-update-stack",
			Pattern: regexp.MustCompile(`\baws\s+cloudformation\s+cancel-update-stack\b`),
			Action:  core.DecisionCaution,
			Reason:  "Cancels in-progress stack update (can leave stack in broken state)",
		},
		BuiltinRule{
			ID:      "builtin:aws:disable-termination-protection",
			Pattern: regexp.MustCompile(`\baws\s+cloudformation\s+update-termination-protection\s+.*--no-enable-termination-protection\b`),
			Action:  core.DecisionCaution,
			Reason:  "Disables stack termination protection",
		},
		BuiltinRule{
			ID:      "builtin:aws:set-stack-policy",
			Pattern: regexp.MustCompile(`\baws\s+cloudformation\s+set-stack-policy\b`),
			Action:  core.DecisionCaution,
			Reason:  "Modifies stack policy (can weaken resource protections)",
		},
		BuiltinRule{
			ID:      "builtin:aws:delete-cloudfront",
			Pattern: regexp.MustCompile(`\baws\s+cloudfront\s+delete-distribution\b`),
			Action:  core.DecisionCaution,
			Reason:  "Deletes CloudFront distribution",
		},
		BuiltinRule{
			ID:      "builtin:aws:delete-elb",
			Pattern: regexp.MustCompile(`\baws\s+elbv2\s+delete-load-balancer\b`),
			Action:  core.DecisionCaution,
			Reason:  "Deletes ALB/NLB",
		},
		BuiltinRule{
			ID:      "builtin:aws:delete-tg",
			Pattern: regexp.MustCompile(`\baws\s+elbv2\s+delete-target-group\b`),
			Action:  core.DecisionCaution,
			Reason:  "Deletes target group",
		},
		BuiltinRule{
			ID:      "builtin:aws:delete-route53",
			Pattern: regexp.MustCompile(`\baws\s+route53\s+delete-hosted-zone\b`),
			Action:  core.DecisionCaution,
			Reason:  "Deletes Route53 hosted zone",
		},
		BuiltinRule{
			ID:      "builtin:aws:change-rrset",
			Pattern: regexp.MustCompile(`\baws\s+route53\s+change-resource-record-sets\b`),
			Action:  core.DecisionCaution,
			Reason:  "Modifies Route53 DNS records",
		},

		// §6.3.2 AWS — IAM & security
		BuiltinRule{
			ID:      "builtin:aws:iam-delete",
			Pattern: regexp.MustCompile(`\baws\s+iam\s+delete-(user|role|policy|group)\b`),
			Action:  core.DecisionCaution,
			Reason:  "Deletes IAM entity",
		},
		BuiltinRule{
			ID:      "builtin:aws:iam-attach",
			Pattern: regexp.MustCompile(`\baws\s+iam\s+(attach|detach|put)-(user|role|group)-policy\b`),
			Action:  core.DecisionCaution,
			Reason:  "Modifies IAM policy attachment",
		},
		BuiltinRule{
			ID:      "builtin:aws:iam-create-key",
			Pattern: regexp.MustCompile(`\baws\s+iam\s+create-access-key\b`),
			Action:  core.DecisionCaution,
			Reason:  "Creates new IAM access key",
		},
		BuiltinRule{
			ID:      "builtin:aws:delete-secret",
			Pattern: regexp.MustCompile(`\baws\s+secretsmanager\s+delete-secret\b`),
			Action:  core.DecisionCaution,
			Reason:  "Deletes secret",
		},
		BuiltinRule{
			ID:      "builtin:aws:kms-disable",
			Pattern: regexp.MustCompile(`\baws\s+kms\s+(disable-key|schedule-key-deletion)\b`),
			Action:  core.DecisionCaution,
			Reason:  "Disables or schedules KMS key deletion",
		},
		BuiltinRule{
			ID:      "builtin:aws:cognito-delete",
			Pattern: regexp.MustCompile(`\baws\s+cognito-idp\s+delete-user-pool\b`),
			Action:  core.DecisionCaution,
			Reason:  "Deletes Cognito user pool",
		},

		// §6.3.2 AWS — Monitoring & logging
		BuiltinRule{
			ID:      "builtin:aws:delete-log-group",
			Pattern: regexp.MustCompile(`\baws\s+logs\s+delete-log-group\b`),
			Action:  core.DecisionCaution,
			Reason:  "Deletes CloudWatch log group",
		},
		BuiltinRule{
			ID:      "builtin:aws:delete-alarm",
			Pattern: regexp.MustCompile(`\baws\s+cloudwatch\s+delete-alarms\b`),
			Action:  core.DecisionCaution,
			Reason:  "Deletes CloudWatch alarms",
		},

		// ===================================================================
		// §6.3.3 Cloud: GCP — Compute & containers
		// ===================================================================
		BuiltinRule{
			ID:      "builtin:gcp:delete-project",
			Pattern: regexp.MustCompile(`\bgcloud\s+projects\s+delete\b`),
			Action:  core.DecisionCaution,
			Reason:  "Deletes entire GCP project",
		},
		BuiltinRule{
			ID:      "builtin:gcp:delete-instance",
			Pattern: regexp.MustCompile(`\bgcloud\s+compute\s+instances\s+delete\b`),
			Action:  core.DecisionCaution,
			Reason:  "Deletes compute instance",
		},
		BuiltinRule{
			ID:      "builtin:gcp:delete-disk",
			Pattern: regexp.MustCompile(`\bgcloud\s+compute\s+disks\s+delete\b`),
			Action:  core.DecisionCaution,
			Reason:  "Deletes persistent disk",
		},
		BuiltinRule{
			ID:      "builtin:gcp:delete-snapshot",
			Pattern: regexp.MustCompile(`\bgcloud\s+compute\s+snapshots\s+delete\b`),
			Action:  core.DecisionCaution,
			Reason:  "Deletes disk snapshot",
		},
		BuiltinRule{
			ID:      "builtin:gcp:delete-image",
			Pattern: regexp.MustCompile(`\bgcloud\s+compute\s+images\s+delete\b`),
			Action:  core.DecisionCaution,
			Reason:  "Deletes compute image",
		},
		BuiltinRule{
			ID:      "builtin:gcp:delete-cluster",
			Pattern: regexp.MustCompile(`\bgcloud\s+container\s+clusters\s+delete\b`),
			Action:  core.DecisionCaution,
			Reason:  "Deletes GKE cluster",
		},
		BuiltinRule{
			ID:      "builtin:gcp:delete-cloud-run",
			Pattern: regexp.MustCompile(`\bgcloud\s+run\s+services\s+delete\b`),
			Action:  core.DecisionCaution,
			Reason:  "Deletes Cloud Run service",
		},
		BuiltinRule{
			ID:      "builtin:gcp:delete-function",
			Pattern: regexp.MustCompile(`\bgcloud\s+functions\s+delete\b`),
			Action:  core.DecisionCaution,
			Reason:  "Deletes Cloud Function",
		},
		BuiltinRule{
			ID:      "builtin:gcp:delete-app-version",
			Pattern: regexp.MustCompile(`\bgcloud\s+app\s+versions\s+delete\b`),
			Action:  core.DecisionCaution,
			Reason:  "Deletes App Engine version",
		},

		// §6.3.3 GCP — Storage & data
		BuiltinRule{
			ID:      "builtin:gcp:delete-bucket",
			Pattern: regexp.MustCompile(`\bgsutil\s+rb\b|gcloud\s+storage\s+buckets\s+delete\b`),
			Action:  core.DecisionCaution,
			Reason:  "Deletes GCS bucket",
		},
		BuiltinRule{
			ID:      "builtin:gcp:gsutil-rm",
			Pattern: regexp.MustCompile(`\bgsutil\s+(-m\s+)?rm\s+(-r\s+)?gs://`),
			Action:  core.DecisionCaution,
			Reason:  "Deletes GCS objects",
		},
		BuiltinRule{
			ID:      "builtin:gcp:delete-artifact",
			Pattern: regexp.MustCompile(`\bgcloud\s+artifacts\s+(repositories|docker\s+images)\s+delete\b`),
			Action:  core.DecisionCaution,
			Reason:  "Deletes Artifact Registry resource",
		},

		// §6.3.3 GCP — Databases & messaging
		BuiltinRule{
			ID:      "builtin:gcp:sql-delete",
			Pattern: regexp.MustCompile(`\bgcloud\s+sql\s+instances\s+delete\b`),
			Action:  core.DecisionCaution,
			Reason:  "Deletes Cloud SQL instance",
		},
		BuiltinRule{
			ID:      "builtin:gcp:delete-dataset",
			Pattern: regexp.MustCompile(`\bgcloud\s+bigquery\s+.*\s+delete\b`),
			Action:  core.DecisionCaution,
			Reason:  "Deletes BigQuery resource",
		},
		BuiltinRule{
			ID:      "builtin:gcp:bq-rm",
			Pattern: regexp.MustCompile(`\bbq\s+rm\b`),
			Action:  core.DecisionCaution,
			Reason:  "Deletes BigQuery table/dataset via bq CLI",
		},
		BuiltinRule{
			ID:      "builtin:gcp:delete-firestore",
			Pattern: regexp.MustCompile(`\bgcloud\s+firestore\s+databases\s+delete\b`),
			Action:  core.DecisionCaution,
			Reason:  "Deletes Firestore database",
		},
		BuiltinRule{
			ID:      "builtin:gcp:delete-spanner",
			Pattern: regexp.MustCompile(`\bgcloud\s+spanner\s+(instances|databases)\s+delete\b`),
			Action:  core.DecisionCaution,
			Reason:  "Deletes Spanner resource",
		},
		BuiltinRule{
			ID:      "builtin:gcp:delete-pubsub",
			Pattern: regexp.MustCompile(`\bgcloud\s+pubsub\s+(topics|subscriptions)\s+delete\b`),
			Action:  core.DecisionCaution,
			Reason:  "Deletes Pub/Sub topic or subscription",
		},
		BuiltinRule{
			ID:      "builtin:gcp:delete-memorystore",
			Pattern: regexp.MustCompile(`\bgcloud\s+redis\s+instances\s+delete\b`),
			Action:  core.DecisionCaution,
			Reason:  "Deletes Memorystore Redis instance",
		},

		// §6.3.3 GCP — Networking & security
		BuiltinRule{
			ID:      "builtin:gcp:delete-network",
			Pattern: regexp.MustCompile(`\bgcloud\s+compute\s+(networks|firewall-rules|routers|addresses)\s+delete\b`),
			Action:  core.DecisionCaution,
			Reason:  "Deletes VPC networking resource",
		},
		BuiltinRule{
			ID:      "builtin:gcp:delete-dns",
			Pattern: regexp.MustCompile(`\bgcloud\s+dns\s+managed-zones\s+delete\b`),
			Action:  core.DecisionCaution,
			Reason:  "Deletes Cloud DNS zone",
		},
		BuiltinRule{
			ID:      "builtin:gcp:iam-binding",
			Pattern: regexp.MustCompile(`\bgcloud\s+.*\s+(add|remove)-iam-policy-binding\b`),
			Action:  core.DecisionCaution,
			Reason:  "Modifies IAM binding",
		},
		BuiltinRule{
			ID:      "builtin:gcp:kms-destroy",
			Pattern: regexp.MustCompile(`\bgcloud\s+kms\s+keys\s+versions\s+destroy\b`),
			Action:  core.DecisionCaution,
			Reason:  "Destroys KMS key version",
		},
		BuiltinRule{
			ID:      "builtin:gcp:delete-sa",
			Pattern: regexp.MustCompile(`\bgcloud\s+iam\s+service-accounts\s+delete\b`),
			Action:  core.DecisionCaution,
			Reason:  "Deletes service account",
		},
		BuiltinRule{
			ID:      "builtin:gcp:create-sa-key",
			Pattern: regexp.MustCompile(`\bgcloud\s+iam\s+service-accounts\s+keys\s+create\b`),
			Action:  core.DecisionCaution,
			Reason:  "Creates service account key",
		},

		// ===================================================================
		// §6.3.4 Cloud: Azure — Compute & containers
		// ===================================================================
		BuiltinRule{
			ID:      "builtin:az:group-delete",
			Pattern: regexp.MustCompile(`\baz\s+group\s+delete\b`),
			Action:  core.DecisionCaution,
			Reason:  "Deletes entire resource group (cascading)",
		},
		BuiltinRule{
			ID:      "builtin:az:vm-delete",
			Pattern: regexp.MustCompile(`\baz\s+vm\s+delete\b`),
			Action:  core.DecisionCaution,
			Reason:  "Deletes virtual machine",
		},
		BuiltinRule{
			ID:      "builtin:az:vmss-delete",
			Pattern: regexp.MustCompile(`\baz\s+vmss\s+delete\b`),
			Action:  core.DecisionCaution,
			Reason:  "Deletes VM scale set",
		},
		BuiltinRule{
			ID:      "builtin:az:aks-delete",
			Pattern: regexp.MustCompile(`\baz\s+aks\s+delete\b`),
			Action:  core.DecisionCaution,
			Reason:  "Deletes AKS cluster",
		},
		BuiltinRule{
			ID:      "builtin:az:webapp-delete",
			Pattern: regexp.MustCompile(`\baz\s+webapp\s+delete\b`),
			Action:  core.DecisionCaution,
			Reason:  "Deletes App Service web app",
		},
		BuiltinRule{
			ID:      "builtin:az:functionapp-delete",
			Pattern: regexp.MustCompile(`\baz\s+functionapp\s+delete\b`),
			Action:  core.DecisionCaution,
			Reason:  "Deletes Azure Function app",
		},
		BuiltinRule{
			ID:      "builtin:az:acr-delete",
			Pattern: regexp.MustCompile(`\baz\s+acr\s+delete\b`),
			Action:  core.DecisionCaution,
			Reason:  "Deletes container registry",
		},

		// §6.3.4 Azure — Storage & data
		BuiltinRule{
			ID:      "builtin:az:storage-delete",
			Pattern: regexp.MustCompile(`\baz\s+storage\s+(account|container|blob)\s+delete\b`),
			Action:  core.DecisionCaution,
			Reason:  "Deletes storage resource",
		},
		BuiltinRule{
			ID:      "builtin:az:cosmosdb-delete",
			Pattern: regexp.MustCompile(`\baz\s+cosmosdb\s+(delete|database\s+delete|collection\s+delete)\b`),
			Action:  core.DecisionCaution,
			Reason:  "Deletes CosmosDB resource",
		},
		BuiltinRule{
			ID:      "builtin:az:sql-delete",
			Pattern: regexp.MustCompile(`\baz\s+sql\s+(server|db)\s+delete\b`),
			Action:  core.DecisionCaution,
			Reason:  "Deletes Azure SQL resource",
		},
		BuiltinRule{
			ID:      "builtin:az:redis-delete",
			Pattern: regexp.MustCompile(`\baz\s+redis\s+delete\b`),
			Action:  core.DecisionCaution,
			Reason:  "Deletes Azure Cache for Redis",
		},
		BuiltinRule{
			ID:      "builtin:az:servicebus-delete",
			Pattern: regexp.MustCompile(`\baz\s+servicebus\s+(namespace|queue|topic)\s+delete\b`),
			Action:  core.DecisionCaution,
			Reason:  "Deletes Service Bus resource",
		},
		BuiltinRule{
			ID:      "builtin:az:eventhubs-delete",
			Pattern: regexp.MustCompile(`\baz\s+eventhubs\s+(namespace|eventhub)\s+delete\b`),
			Action:  core.DecisionCaution,
			Reason:  "Deletes Event Hubs resource",
		},

		// §6.3.4 Azure — Networking & security
		BuiltinRule{
			ID:      "builtin:az:network-delete",
			Pattern: regexp.MustCompile(`\baz\s+network\s+(vnet|nsg|public-ip|lb|application-gateway)\s+delete\b`),
			Action:  core.DecisionCaution,
			Reason:  "Deletes networking resource",
		},
		BuiltinRule{
			ID:      "builtin:az:dns-delete",
			Pattern: regexp.MustCompile(`\baz\s+network\s+dns\s+zone\s+delete\b`),
			Action:  core.DecisionCaution,
			Reason:  "Deletes DNS zone",
		},
		BuiltinRule{
			ID:      "builtin:az:keyvault-delete",
			Pattern: regexp.MustCompile(`\baz\s+keyvault\s+delete\b`),
			Action:  core.DecisionCaution,
			Reason:  "Deletes Key Vault",
		},
		BuiltinRule{
			ID:      "builtin:az:keyvault-secret-delete",
			Pattern: regexp.MustCompile(`\baz\s+keyvault\s+secret\s+delete\b`),
			Action:  core.DecisionCaution,
			Reason:  "Deletes Key Vault secret",
		},
		BuiltinRule{
			ID:      "builtin:az:ad-delete",
			Pattern: regexp.MustCompile(`\baz\s+ad\s+(app|sp|group)\s+delete\b`),
			Action:  core.DecisionCaution,
			Reason:  "Deletes Azure AD entity",
		},
		BuiltinRule{
			ID:      "builtin:az:role-assignment",
			Pattern: regexp.MustCompile(`\baz\s+role\s+assignment\s+(create|delete)\b`),
			Action:  core.DecisionCaution,
			Reason:  "Modifies role assignment",
		},

		// §6.3.4 Azure — Monitoring
		BuiltinRule{
			ID:      "builtin:az:monitor-delete",
			Pattern: regexp.MustCompile(`\baz\s+monitor\s+.*\s+delete\b`),
			Action:  core.DecisionCaution,
			Reason:  "Deletes monitoring resource",
		},

		// ===================================================================
		// §6.3.5 Infrastructure as Code — Terraform / OpenTofu
		// ===================================================================
		BuiltinRule{
			ID:      "builtin:terraform:destroy",
			Pattern: regexp.MustCompile(`\b(terraform|tofu)\s+destroy\b`),
			Action:  core.DecisionCaution,
			Reason:  "Destroys Terraform-managed infrastructure",
		},
		BuiltinRule{
			ID:      "builtin:terraform:apply",
			Pattern: regexp.MustCompile(`\b(terraform|tofu)\s+apply\b`),
			Action:  core.DecisionCaution,
			Reason:  "Applies Terraform changes",
		},
		BuiltinRule{
			ID:      "builtin:terraform:plan-destroy",
			Pattern: regexp.MustCompile(`\b(terraform|tofu)\s+plan\s+.*-destroy\b`),
			Action:  core.DecisionCaution,
			Reason:  "Plans a destroy operation",
		},
		BuiltinRule{
			ID:      "builtin:terraform:taint",
			Pattern: regexp.MustCompile(`\b(terraform|tofu)\s+taint\b`),
			Action:  core.DecisionCaution,
			Reason:  "Marks resource for recreation",
		},
		BuiltinRule{
			ID:      "builtin:terraform:state-rm",
			Pattern: regexp.MustCompile(`\b(terraform|tofu)\s+state\s+rm\b`),
			Action:  core.DecisionCaution,
			Reason:  "Removes resource from state",
		},
		BuiltinRule{
			ID:      "builtin:terraform:state-mv",
			Pattern: regexp.MustCompile(`\b(terraform|tofu)\s+state\s+mv\b`),
			Action:  core.DecisionCaution,
			Reason:  "Moves resource in state",
		},
		BuiltinRule{
			ID:      "builtin:terraform:force-unlock",
			Pattern: regexp.MustCompile(`\b(terraform|tofu)\s+force-unlock\b`),
			Action:  core.DecisionCaution,
			Reason:  "Force-unlocks state lock",
		},
		BuiltinRule{
			ID:      "builtin:terraform:workspace-delete",
			Pattern: regexp.MustCompile(`\b(terraform|tofu)\s+workspace\s+delete\b`),
			Action:  core.DecisionCaution,
			Reason:  "Deletes Terraform workspace",
		},
		BuiltinRule{
			ID:      "builtin:terraform:import",
			Pattern: regexp.MustCompile(`\b(terraform|tofu)\s+import\b`),
			Action:  core.DecisionCaution,
			Reason:  "Imports existing resource into state",
		},

		// §6.3.5 AWS CDK
		BuiltinRule{
			ID:      "builtin:cdk:destroy",
			Pattern: regexp.MustCompile(`\bcdk\s+destroy\b`),
			Action:  core.DecisionCaution,
			Reason:  "Destroys CDK stack",
		},
		BuiltinRule{
			ID:      "builtin:cdk:deploy-force",
			Pattern: regexp.MustCompile(`\bcdk\s+deploy\s+.*--force\b`),
			Action:  core.DecisionCaution,
			Reason:  "Force-deploys CDK stack (bypasses changeset)",
		},
		BuiltinRule{
			ID:      "builtin:cdk:deploy",
			Pattern: regexp.MustCompile(`\bcdk\s+deploy\b`),
			Action:  core.DecisionCaution,
			Reason:  "Deploys CDK stack",
		},

		// §6.3.5 Pulumi
		BuiltinRule{
			ID:      "builtin:pulumi:destroy",
			Pattern: regexp.MustCompile(`\bpulumi\s+destroy\b`),
			Action:  core.DecisionCaution,
			Reason:  "Destroys Pulumi stack",
		},
		BuiltinRule{
			ID:      "builtin:pulumi:up",
			Pattern: regexp.MustCompile(`\bpulumi\s+up\b`),
			Action:  core.DecisionCaution,
			Reason:  "Applies Pulumi changes",
		},
		BuiltinRule{
			ID:      "builtin:pulumi:up-yes",
			Pattern: regexp.MustCompile(`\bpulumi\s+up\s+.*(-y|--yes)\b`),
			Action:  core.DecisionCaution,
			Reason:  "Applies Pulumi changes non-interactively",
		},
		BuiltinRule{
			ID:      "builtin:pulumi:refresh-yes",
			Pattern: regexp.MustCompile(`\bpulumi\s+refresh\s+.*(-y|--yes)\b`),
			Action:  core.DecisionCaution,
			Reason:  "Refreshes state non-interactively",
		},
		BuiltinRule{
			ID:      "builtin:pulumi:cancel",
			Pattern: regexp.MustCompile(`\bpulumi\s+cancel\b`),
			Action:  core.DecisionCaution,
			Reason:  "Cancels in-progress update",
		},
		BuiltinRule{
			ID:      "builtin:pulumi:stack-rm",
			Pattern: regexp.MustCompile(`\bpulumi\s+stack\s+rm\b`),
			Action:  core.DecisionCaution,
			Reason:  "Removes Pulumi stack",
		},
		BuiltinRule{
			ID:      "builtin:pulumi:state-delete",
			Pattern: regexp.MustCompile(`\bpulumi\s+state\s+delete\b`),
			Action:  core.DecisionCaution,
			Reason:  "Deletes resource from state",
		},

		// §6.3.5 Ansible
		BuiltinRule{
			ID:      "builtin:ansible:playbook",
			Pattern: regexp.MustCompile(`\bansible-playbook\b`),
			Action:  core.DecisionCaution,
			Reason:  "Runs Ansible playbook (arbitrary remote execution)",
		},
		BuiltinRule{
			ID:      "builtin:ansible:galaxy-remove",
			Pattern: regexp.MustCompile(`\bansible-galaxy\s+.*remove\b`),
			Action:  core.DecisionCaution,
			Reason:  "Removes Ansible role/collection",
		},

		// ===================================================================
		// §6.3.6 Kubernetes
		// ===================================================================
		BuiltinRule{
			ID:      "builtin:k8s:delete",
			Pattern: regexp.MustCompile(`\bkubectl\s+delete\b`),
			Action:  core.DecisionCaution,
			Reason:  "Deletes Kubernetes resources",
		},
		BuiltinRule{
			ID:      "builtin:k8s:drain",
			Pattern: regexp.MustCompile(`\bkubectl\s+drain\b`),
			Action:  core.DecisionCaution,
			Reason:  "Drains a node",
		},
		BuiltinRule{
			ID:      "builtin:k8s:cordon",
			Pattern: regexp.MustCompile(`\bkubectl\s+cordon\b`),
			Action:  core.DecisionCaution,
			Reason:  "Cordons a node",
		},
		BuiltinRule{
			ID:      "builtin:k8s:replace-force",
			Pattern: regexp.MustCompile(`\bkubectl\s+replace\s+--force\b`),
			Action:  core.DecisionCaution,
			Reason:  "Force-replaces resources",
		},
		BuiltinRule{
			ID:      "builtin:k8s:rollout-undo",
			Pattern: regexp.MustCompile(`\bkubectl\s+rollout\s+undo\b`),
			Action:  core.DecisionCaution,
			Reason:  "Rolls back deployment",
		},
		BuiltinRule{
			ID:      "builtin:helm:uninstall",
			Pattern: regexp.MustCompile(`\bhelm\s+(uninstall|delete)\b`),
			Action:  core.DecisionCaution,
			Reason:  "Uninstalls Helm release",
		},

		// ===================================================================
		// §6.3.7 Containers
		// ===================================================================
		BuiltinRule{
			ID:      "builtin:docker:system-prune",
			Pattern: regexp.MustCompile(`\bdocker\s+system\s+prune\b`),
			Action:  core.DecisionCaution,
			Reason:  "Prunes all unused Docker data",
		},
		BuiltinRule{
			ID:      "builtin:docker:volume-rm",
			Pattern: regexp.MustCompile(`\bdocker\s+volume\s+rm\b`),
			Action:  core.DecisionCaution,
			Reason:  "Removes Docker volumes",
		},
		BuiltinRule{
			ID:      "builtin:docker:rm-force",
			Pattern: regexp.MustCompile(`\bdocker\s+rm\s+-f\b`),
			Action:  core.DecisionCaution,
			Reason:  "Force-removes containers",
		},
		BuiltinRule{
			ID:      "builtin:docker:rmi",
			Pattern: regexp.MustCompile(`\bdocker\s+rmi\b`),
			Action:  core.DecisionCaution,
			Reason:  "Removes Docker images",
		},

		// ===================================================================
		// §6.3.8 Databases
		// ===================================================================
		BuiltinRule{
			ID:      "builtin:db:drop-database",
			Pattern: regexp.MustCompile(`\bDROP\s+DATABASE\b`),
			Action:  core.DecisionCaution,
			Reason:  "Drops entire database",
		},
		BuiltinRule{
			ID:      "builtin:db:drop-table",
			Pattern: regexp.MustCompile(`\bDROP\s+TABLE\b`),
			Action:  core.DecisionCaution,
			Reason:  "Drops table",
		},
		BuiltinRule{
			ID:      "builtin:db:truncate",
			Pattern: regexp.MustCompile(`\bTRUNCATE\s+TABLE\b`),
			Action:  core.DecisionCaution,
			Reason:  "Truncates table",
		},
		BuiltinRule{
			ID:      "builtin:db:delete-no-where",
			Pattern: regexp.MustCompile(`\bDELETE\s+FROM\s+\S+\s*;`),
			Action:  core.DecisionCaution,
			Reason:  "DELETE without WHERE clause",
		},
		BuiltinRule{
			ID:      "builtin:db:alter-drop",
			Pattern: regexp.MustCompile(`\bALTER\s+TABLE\s+.*\bDROP\b`),
			Action:  core.DecisionCaution,
			Reason:  "Drops column or constraint",
		},
		BuiltinRule{
			ID:      "builtin:db:mongo-drop",
			Pattern: regexp.MustCompile(`\b\.drop(Database|Collection)\(\)`),
			Action:  core.DecisionCaution,
			Reason:  "MongoDB drop operations",
		},

		// ===================================================================
		// §6.3.9 Remote execution
		// ===================================================================
		BuiltinRule{
			ID:      "builtin:ssh:remote-cmd",
			Pattern: regexp.MustCompile(`\bssh\s+\S+\s+.+`),
			Action:  core.DecisionCaution,
			Reason:  "SSH with remote command -- inner command not fully visible",
		},
		BuiltinRule{
			ID:      "builtin:scp:copy",
			Pattern: regexp.MustCompile(`\bscp\b.*:`),
			Action:  core.DecisionCaution,
			Reason:  "SCP to/from remote host",
		},
		BuiltinRule{
			ID:      "builtin:rsync:delete",
			Pattern: regexp.MustCompile(`\brsync\s+.*--delete\b`),
			Action:  core.DecisionCaution,
			Reason:  "Rsync with delete flag",
		},

		// ===================================================================
		// §6.3.10 Database CLIs
		// ===================================================================
		BuiltinRule{
			ID:      "builtin:db:psql-cmd",
			Pattern: regexp.MustCompile(`\bpsql\s+.*(-c|--command)\b`),
			Action:  core.DecisionCaution,
			Reason:  "psql with inline command",
		},
		BuiltinRule{
			ID:      "builtin:db:mysql-exec",
			Pattern: regexp.MustCompile(`\bmysql\s+.*(-e|--execute)\b`),
			Action:  core.DecisionCaution,
			Reason:  "mysql with inline command",
		},
		BuiltinRule{
			ID:      "builtin:db:mongo-eval",
			Pattern: regexp.MustCompile(`\bmongo\w*\s+.*--eval\b`),
			Action:  core.DecisionCaution,
			Reason:  "mongo with inline eval",
		},
		BuiltinRule{
			ID:      "builtin:db:redis-flush",
			Pattern: regexp.MustCompile(`\bredis-cli\s+.*(FLUSHALL|FLUSHDB)\b`),
			Action:  core.DecisionCaution,
			Reason:  "Redis flush operations",
		},
		BuiltinRule{
			ID:      "builtin:db:redis-del",
			Pattern: regexp.MustCompile(`\bredis-cli\s+.*\bDEL\b`),
			Action:  core.DecisionCaution,
			Reason:  "Redis delete operations",
		},

		// ===================================================================
		// §6.3.11 System services
		// ===================================================================
		BuiltinRule{
			ID:      "builtin:sys:systemctl-stop",
			Pattern: regexp.MustCompile(`\bsystemctl\s+(stop|disable|mask)\b`),
			Action:  core.DecisionCaution,
			Reason:  "Stops or disables system services",
		},
		BuiltinRule{
			ID:      "builtin:sys:launchctl-unload",
			Pattern: regexp.MustCompile(`\blaunchctl\s+(unload|bootout|disable)\b`),
			Action:  core.DecisionCaution,
			Reason:  "Unloads macOS services",
		},
		BuiltinRule{
			ID:      "builtin:sys:kill-pid",
			Pattern: regexp.MustCompile(`\bkill\s+(-9\s+)?1\b`),
			Action:  core.DecisionCaution,
			Reason:  "Killing PID 1 (init/systemd)",
		},
		BuiltinRule{
			ID:      "builtin:sys:pkill-force",
			Pattern: regexp.MustCompile(`\bpkill\s+.*-9\b`),
			Action:  core.DecisionCaution,
			Reason:  "Force-killing processes",
		},
		BuiltinRule{
			ID:      "builtin:sys:killall",
			Pattern: regexp.MustCompile(`\bkillall\b`),
			Action:  core.DecisionCaution,
			Reason:  "Killing processes by name",
		},
		BuiltinRule{
			ID:      "builtin:sys:iptables-flush",
			Pattern: regexp.MustCompile(`\biptables\s+-F\b`),
			Action:  core.DecisionCaution,
			Reason:  "Flushing all firewall rules",
		},
		BuiltinRule{
			ID:      "builtin:sys:truncate-file",
			Pattern: regexp.MustCompile(`\btruncate\s+.*-s\s*0\b`),
			Action:  core.DecisionCaution,
			Reason:  "Truncating files to zero bytes",
		},
	)
}
