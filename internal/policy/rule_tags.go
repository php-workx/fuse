package policy

// init registers tags and keywords for all builtin rules.
// This is separate from rule definitions to avoid touching the large rule files.
func init() {
	applyRuleMetadata(builtinRuleTags())
}

func builtinRuleTags() map[string]ruleMetadata { //nolint:funlen // tag registry is intentionally large
	return map[string]ruleMetadata{
		// === Git ===
		"builtin:git:reset-hard":       {tags: []string{"git", "vcs"}, keywords: []string{"git"}},
		"builtin:git:clean":            {tags: []string{"git", "vcs"}, keywords: []string{"git"}},
		"builtin:git:push-force":       {tags: []string{"git", "vcs"}, keywords: []string{"git"}},
		"builtin:git:push-force-lease": {tags: []string{"git", "vcs"}, keywords: []string{"git"}},
		"builtin:git:stash-clear":      {tags: []string{"git", "vcs"}, keywords: []string{"git"}},
		"builtin:git:stash-drop":       {tags: []string{"git", "vcs"}, keywords: []string{"git"}},
		"builtin:git:branch-D":         {tags: []string{"git", "vcs"}, keywords: []string{"git"}},
		"builtin:git:checkout-dot":     {tags: []string{"git", "vcs"}, keywords: []string{"git"}},
		"builtin:git:restore-worktree": {tags: []string{"git", "vcs"}, keywords: []string{"git"}},

		// === AWS ===
		"builtin:aws:terminate-instances":  {tags: []string{"aws", "cloud", "ec2"}, keywords: []string{"aws"}},
		"builtin:aws:stop-instances":       {tags: []string{"aws", "cloud", "ec2"}, keywords: []string{"aws"}},
		"builtin:aws:delete-snapshot":      {tags: []string{"aws", "cloud", "ec2"}, keywords: []string{"aws"}},
		"builtin:aws:delete-volume":        {tags: []string{"aws", "cloud", "ec2"}, keywords: []string{"aws"}},
		"builtin:aws:delete-vpc":           {tags: []string{"aws", "cloud", "ec2"}, keywords: []string{"aws"}},
		"builtin:aws:delete-subnet":        {tags: []string{"aws", "cloud", "ec2"}, keywords: []string{"aws"}},
		"builtin:aws:delete-sg":            {tags: []string{"aws", "cloud", "ec2"}, keywords: []string{"aws"}},
		"builtin:aws:delete-keypair":       {tags: []string{"aws", "cloud", "ec2"}, keywords: []string{"aws"}},
		"builtin:aws:deregister-ami":       {tags: []string{"aws", "cloud", "ec2"}, keywords: []string{"aws"}},
		"builtin:aws:modify-sg-ingress":    {tags: []string{"aws", "cloud", "ec2"}, keywords: []string{"aws"}},
		"builtin:aws:delete-ecs-service":   {tags: []string{"aws", "cloud", "ecs"}, keywords: []string{"aws"}},
		"builtin:aws:delete-ecs-cluster":   {tags: []string{"aws", "cloud", "ecs"}, keywords: []string{"aws"}},
		"builtin:aws:deregister-taskdef":   {tags: []string{"aws", "cloud", "ecs"}, keywords: []string{"aws"}},
		"builtin:aws:delete-eks-cluster":   {tags: []string{"aws", "cloud", "eks"}, keywords: []string{"aws"}},
		"builtin:aws:delete-eks-nodegroup": {tags: []string{"aws", "cloud", "eks"}, keywords: []string{"aws"}},
		"builtin:aws:delete-bucket":        {tags: []string{"aws", "cloud", "s3"}, keywords: []string{"aws"}},
		"builtin:aws:s3-rm":                {tags: []string{"aws", "cloud", "s3"}, keywords: []string{"aws"}},
		"builtin:aws:delete-ecr-repo":      {tags: []string{"aws", "cloud", "ecr"}, keywords: []string{"aws"}},
		"builtin:aws:ecr-batch-delete":     {tags: []string{"aws", "cloud", "ecr"}, keywords: []string{"aws"}},
		"builtin:aws:delete-db":            {tags: []string{"aws", "cloud", "rds"}, keywords: []string{"aws"}},
		"builtin:aws:delete-table":         {tags: []string{"aws", "cloud", "dynamodb"}, keywords: []string{"aws"}},
		"builtin:aws:delete-elasticache":   {tags: []string{"aws", "cloud", "elasticache"}, keywords: []string{"aws"}},
		"builtin:aws:delete-kinesis":       {tags: []string{"aws", "cloud", "kinesis"}, keywords: []string{"aws"}},
		"builtin:aws:delete-function":      {tags: []string{"aws", "cloud", "lambda"}, keywords: []string{"aws"}},
		"builtin:aws:delete-rest-api":      {tags: []string{"aws", "cloud", "apigateway"}, keywords: []string{"aws"}},
		"builtin:aws:delete-apigw-v2":      {tags: []string{"aws", "cloud", "apigateway"}, keywords: []string{"aws"}},
		"builtin:aws:delete-sfn":           {tags: []string{"aws", "cloud", "stepfunctions"}, keywords: []string{"aws"}},
		"builtin:aws:delete-eventbridge":   {tags: []string{"aws", "cloud", "eventbridge"}, keywords: []string{"aws"}},
		"builtin:aws:delete-sqs":           {tags: []string{"aws", "cloud", "sqs"}, keywords: []string{"aws"}},
		"builtin:aws:purge-sqs":            {tags: []string{"aws", "cloud", "sqs"}, keywords: []string{"aws"}},
		"builtin:aws:delete-sns":           {tags: []string{"aws", "cloud", "sns"}, keywords: []string{"aws"}},
		"builtin:aws:delete-ses-identity":  {tags: []string{"aws", "cloud", "ses"}, keywords: []string{"aws"}},
		"builtin:aws:delete-stack":         {tags: []string{"aws", "cloud", "cloudformation"}, keywords: []string{"aws"}},
		"builtin:aws:delete-cloudfront":    {tags: []string{"aws", "cloud", "cloudfront"}, keywords: []string{"aws"}},
		"builtin:aws:delete-elb":           {tags: []string{"aws", "cloud", "elb"}, keywords: []string{"aws"}},
		"builtin:aws:delete-tg":            {tags: []string{"aws", "cloud", "elb"}, keywords: []string{"aws"}},
		"builtin:aws:delete-route53":       {tags: []string{"aws", "cloud", "route53"}, keywords: []string{"aws"}},
		"builtin:aws:change-rrset":         {tags: []string{"aws", "cloud", "route53"}, keywords: []string{"aws"}},
		"builtin:aws:iam-delete":           {tags: []string{"aws", "cloud", "iam"}, keywords: []string{"aws"}},
		"builtin:aws:iam-attach":           {tags: []string{"aws", "cloud", "iam"}, keywords: []string{"aws"}},
		"builtin:aws:iam-create-key":       {tags: []string{"aws", "cloud", "iam"}, keywords: []string{"aws"}},
		"builtin:aws:delete-secret":        {tags: []string{"aws", "cloud", "secrets"}, keywords: []string{"aws"}},
		"builtin:aws:kms-disable":          {tags: []string{"aws", "cloud", "kms"}, keywords: []string{"aws"}},
		"builtin:aws:cognito-delete":       {tags: []string{"aws", "cloud", "cognito"}, keywords: []string{"aws"}},
		"builtin:aws:delete-log-group":     {tags: []string{"aws", "cloud", "cloudwatch"}, keywords: []string{"aws"}},
		"builtin:aws:delete-alarm":         {tags: []string{"aws", "cloud", "cloudwatch"}, keywords: []string{"aws"}},

		// === GCP ===
		"builtin:gcp:delete-project":     {tags: []string{"gcp", "cloud"}, keywords: []string{"gcloud"}},
		"builtin:gcp:delete-instance":    {tags: []string{"gcp", "cloud", "compute"}, keywords: []string{"gcloud"}},
		"builtin:gcp:delete-disk":        {tags: []string{"gcp", "cloud", "compute"}, keywords: []string{"gcloud"}},
		"builtin:gcp:delete-snapshot":    {tags: []string{"gcp", "cloud", "compute"}, keywords: []string{"gcloud"}},
		"builtin:gcp:delete-image":       {tags: []string{"gcp", "cloud", "compute"}, keywords: []string{"gcloud"}},
		"builtin:gcp:delete-cluster":     {tags: []string{"gcp", "cloud", "gke"}, keywords: []string{"gcloud"}},
		"builtin:gcp:delete-cloud-run":   {tags: []string{"gcp", "cloud", "cloudrun"}, keywords: []string{"gcloud"}},
		"builtin:gcp:delete-function":    {tags: []string{"gcp", "cloud", "functions"}, keywords: []string{"gcloud"}},
		"builtin:gcp:delete-app-version": {tags: []string{"gcp", "cloud", "appengine"}, keywords: []string{"gcloud"}},
		"builtin:gcp:delete-bucket":      {tags: []string{"gcp", "cloud", "gcs"}, keywords: []string{"gsutil", "gcloud"}},
		"builtin:gcp:gsutil-rm":          {tags: []string{"gcp", "cloud", "gcs"}, keywords: []string{"gsutil"}},
		"builtin:gcp:delete-artifact":    {tags: []string{"gcp", "cloud", "artifact-registry"}, keywords: []string{"gcloud"}},
		"builtin:gcp:sql-delete":         {tags: []string{"gcp", "cloud", "cloudsql"}, keywords: []string{"gcloud"}},
		"builtin:gcp:delete-dataset":     {tags: []string{"gcp", "cloud", "bigquery"}, keywords: []string{"gcloud"}},
		"builtin:gcp:bq-rm":              {tags: []string{"gcp", "cloud", "bigquery"}, keywords: []string{"bq"}},
		"builtin:gcp:delete-firestore":   {tags: []string{"gcp", "cloud", "firestore"}, keywords: []string{"gcloud"}},
		"builtin:gcp:delete-spanner":     {tags: []string{"gcp", "cloud", "spanner"}, keywords: []string{"gcloud"}},
		"builtin:gcp:delete-pubsub":      {tags: []string{"gcp", "cloud", "pubsub"}, keywords: []string{"gcloud"}},
		"builtin:gcp:delete-memorystore": {tags: []string{"gcp", "cloud", "redis"}, keywords: []string{"gcloud"}},
		"builtin:gcp:delete-network":     {tags: []string{"gcp", "cloud", "networking"}, keywords: []string{"gcloud"}},
		"builtin:gcp:delete-dns":         {tags: []string{"gcp", "cloud", "dns"}, keywords: []string{"gcloud"}},
		"builtin:gcp:iam-binding":        {tags: []string{"gcp", "cloud", "iam"}, keywords: []string{"gcloud"}},
		"builtin:gcp:kms-destroy":        {tags: []string{"gcp", "cloud", "kms"}, keywords: []string{"gcloud"}},
		"builtin:gcp:delete-sa":          {tags: []string{"gcp", "cloud", "iam"}, keywords: []string{"gcloud"}},
		"builtin:gcp:create-sa-key":      {tags: []string{"gcp", "cloud", "iam"}, keywords: []string{"gcloud"}},

		// === Azure ===
		"builtin:az:group-delete":           {tags: []string{"azure", "cloud"}, keywords: []string{"az"}},
		"builtin:az:vm-delete":              {tags: []string{"azure", "cloud", "vm"}, keywords: []string{"az"}},
		"builtin:az:vmss-delete":            {tags: []string{"azure", "cloud", "vmss"}, keywords: []string{"az"}},
		"builtin:az:aks-delete":             {tags: []string{"azure", "cloud", "aks"}, keywords: []string{"az"}},
		"builtin:az:webapp-delete":          {tags: []string{"azure", "cloud", "appservice"}, keywords: []string{"az"}},
		"builtin:az:functionapp-delete":     {tags: []string{"azure", "cloud", "functions"}, keywords: []string{"az"}},
		"builtin:az:acr-delete":             {tags: []string{"azure", "cloud", "acr"}, keywords: []string{"az"}},
		"builtin:az:storage-delete":         {tags: []string{"azure", "cloud", "storage"}, keywords: []string{"az"}},
		"builtin:az:cosmosdb-delete":        {tags: []string{"azure", "cloud", "cosmosdb"}, keywords: []string{"az"}},
		"builtin:az:sql-delete":             {tags: []string{"azure", "cloud", "sql"}, keywords: []string{"az"}},
		"builtin:az:redis-delete":           {tags: []string{"azure", "cloud", "redis"}, keywords: []string{"az"}},
		"builtin:az:servicebus-delete":      {tags: []string{"azure", "cloud", "servicebus"}, keywords: []string{"az"}},
		"builtin:az:eventhubs-delete":       {tags: []string{"azure", "cloud", "eventhubs"}, keywords: []string{"az"}},
		"builtin:az:network-delete":         {tags: []string{"azure", "cloud", "networking"}, keywords: []string{"az"}},
		"builtin:az:dns-delete":             {tags: []string{"azure", "cloud", "dns"}, keywords: []string{"az"}},
		"builtin:az:keyvault-delete":        {tags: []string{"azure", "cloud", "keyvault"}, keywords: []string{"az"}},
		"builtin:az:keyvault-secret-delete": {tags: []string{"azure", "cloud", "keyvault"}, keywords: []string{"az"}},
		"builtin:az:ad-delete":              {tags: []string{"azure", "cloud", "ad"}, keywords: []string{"az"}},
		"builtin:az:role-assignment":        {tags: []string{"azure", "cloud", "iam"}, keywords: []string{"az"}},
		"builtin:az:monitor-delete":         {tags: []string{"azure", "cloud", "monitoring"}, keywords: []string{"az"}},

		// === Terraform / IaC ===
		"builtin:terraform:destroy":          {tags: []string{"terraform", "iac"}, keywords: []string{"terraform", "tofu"}},
		"builtin:terraform:apply":            {tags: []string{"terraform", "iac"}, keywords: []string{"terraform", "tofu"}},
		"builtin:terraform:plan-destroy":     {tags: []string{"terraform", "iac"}, keywords: []string{"terraform", "tofu"}},
		"builtin:terraform:taint":            {tags: []string{"terraform", "iac"}, keywords: []string{"terraform", "tofu"}},
		"builtin:terraform:state-rm":         {tags: []string{"terraform", "iac"}, keywords: []string{"terraform", "tofu"}},
		"builtin:terraform:state-mv":         {tags: []string{"terraform", "iac"}, keywords: []string{"terraform", "tofu"}},
		"builtin:terraform:force-unlock":     {tags: []string{"terraform", "iac"}, keywords: []string{"terraform", "tofu"}},
		"builtin:terraform:workspace-delete": {tags: []string{"terraform", "iac"}, keywords: []string{"terraform", "tofu"}},
		"builtin:terraform:import":           {tags: []string{"terraform", "iac"}, keywords: []string{"terraform", "tofu"}},
		"builtin:cdk:destroy":                {tags: []string{"cdk", "iac", "aws"}, keywords: []string{"cdk"}},
		"builtin:cdk:deploy-force":           {tags: []string{"cdk", "iac", "aws"}, keywords: []string{"cdk"}},
		"builtin:cdk:deploy":                 {tags: []string{"cdk", "iac", "aws"}, keywords: []string{"cdk"}},

		// === Pulumi ===
		"builtin:pulumi:destroy":      {tags: []string{"pulumi", "iac"}, keywords: []string{"pulumi"}},
		"builtin:pulumi:up":           {tags: []string{"pulumi", "iac"}, keywords: []string{"pulumi"}},
		"builtin:pulumi:up-yes":       {tags: []string{"pulumi", "iac"}, keywords: []string{"pulumi"}},
		"builtin:pulumi:refresh-yes":  {tags: []string{"pulumi", "iac"}, keywords: []string{"pulumi"}},
		"builtin:pulumi:cancel":       {tags: []string{"pulumi", "iac"}, keywords: []string{"pulumi"}},
		"builtin:pulumi:stack-rm":     {tags: []string{"pulumi", "iac"}, keywords: []string{"pulumi"}},
		"builtin:pulumi:state-delete": {tags: []string{"pulumi", "iac"}, keywords: []string{"pulumi"}},

		// === Kubernetes ===
		"builtin:k8s:delete":        {tags: []string{"kubernetes", "k8s"}, keywords: []string{"kubectl"}},
		"builtin:k8s:drain":         {tags: []string{"kubernetes", "k8s"}, keywords: []string{"kubectl"}},
		"builtin:k8s:cordon":        {tags: []string{"kubernetes", "k8s"}, keywords: []string{"kubectl"}},
		"builtin:k8s:replace-force": {tags: []string{"kubernetes", "k8s"}, keywords: []string{"kubectl"}},
		"builtin:k8s:rollout-undo":  {tags: []string{"kubernetes", "k8s"}, keywords: []string{"kubectl"}},
		"builtin:helm:uninstall":    {tags: []string{"kubernetes", "helm"}, keywords: []string{"helm"}},

		// === Containers ===
		"builtin:docker:system-prune": {tags: []string{"docker", "container"}, keywords: []string{"docker"}},
		"builtin:docker:volume-rm":    {tags: []string{"docker", "container"}, keywords: []string{"docker"}},
		"builtin:docker:rm-force":     {tags: []string{"docker", "container"}, keywords: []string{"docker"}},
		"builtin:docker:rmi":          {tags: []string{"docker", "container"}, keywords: []string{"docker"}},

		// === Databases ===
		"builtin:db:drop-database":   {tags: []string{"database", "sql"}, keywords: []string{"drop"}},
		"builtin:db:drop-table":      {tags: []string{"database", "sql"}, keywords: []string{"drop"}},
		"builtin:db:truncate":        {tags: []string{"database", "sql"}, keywords: []string{"truncate"}},
		"builtin:db:delete-no-where": {tags: []string{"database", "sql"}, keywords: []string{"delete"}},
		"builtin:db:alter-drop":      {tags: []string{"database", "sql"}, keywords: []string{"alter"}},
		"builtin:db:mongo-drop":      {tags: []string{"database", "mongodb"}, keywords: []string{"drop"}},
		"builtin:db:psql-cmd":        {tags: []string{"database", "postgresql"}, keywords: []string{"psql"}},
		"builtin:db:mysql-exec":      {tags: []string{"database", "mysql"}, keywords: []string{"mysql"}},
		"builtin:db:mongo-eval":      {tags: []string{"database", "mongodb"}, keywords: []string{"mongo"}},
		"builtin:db:redis-flush":     {tags: []string{"database", "redis"}, keywords: []string{"redis"}},
		"builtin:db:redis-del":       {tags: []string{"database", "redis"}, keywords: []string{"redis"}},

		// === System ===
		"builtin:sys:systemctl-stop":   {tags: []string{"system", "services"}, keywords: []string{"systemctl"}},
		"builtin:sys:launchctl-unload": {tags: []string{"system", "services"}, keywords: []string{"launchctl"}},
		"builtin:sys:kill-pid":         {tags: []string{"system", "process"}, keywords: []string{"kill"}},
		"builtin:sys:pkill-force":      {tags: []string{"system", "process"}, keywords: []string{"pkill"}},
		"builtin:sys:killall":          {tags: []string{"system", "process"}, keywords: []string{"killall"}},
		"builtin:sys:iptables-flush":   {tags: []string{"system", "firewall"}, keywords: []string{"iptables"}},
		"builtin:sys:truncate-file":    {tags: []string{"system", "filesystem"}, keywords: []string{"truncate"}},

		// === Remote ===
		"builtin:ssh:remote-cmd": {tags: []string{"remote", "ssh"}, keywords: []string{"ssh"}},
		"builtin:scp:copy":       {tags: []string{"remote", "scp"}, keywords: []string{"scp"}},
		"builtin:rsync:delete":   {tags: []string{"remote", "rsync"}, keywords: []string{"rsync"}},

		// === PaaS ===
		"builtin:paas:heroku-destroy": {tags: []string{"paas", "heroku"}, keywords: []string{"heroku"}},
		"builtin:paas:fly-destroy":    {tags: []string{"paas", "fly"}, keywords: []string{"fly"}},
		"builtin:paas:railway-delete": {tags: []string{"paas", "railway"}, keywords: []string{"railway"}},

		// === Ansible ===
		"builtin:ansible:playbook":      {tags: []string{"ansible", "iac"}, keywords: []string{"ansible"}},
		"builtin:ansible:galaxy-remove": {tags: []string{"ansible", "iac"}, keywords: []string{"ansible"}},
	}
}

type ruleMetadata struct {
	tags     []string
	keywords []string
}

// applyRuleMetadata sets Tags and Keywords on BuiltinRules by ID.
func applyRuleMetadata(meta map[string]ruleMetadata) {
	for i := range BuiltinRules {
		if m, ok := meta[BuiltinRules[i].ID]; ok {
			BuiltinRules[i].Tags = m.tags
			BuiltinRules[i].Keywords = m.keywords
		}
	}
}
