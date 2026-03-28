package core

import "testing"

func TestUnconditionalSafe(t *testing.T) {
	// All single-word commands from spec §6.5 should be unconditionally safe.
	safeCommands := []string{
		// File reading / inspection
		"ls", "cat", "head", "tail", "less", "more", "file", "stat", "wc",
		"md5sum", "sha256sum", "sha1sum", "cksum", "du", "df",
		// Text processing
		"echo", "printf", "grep", "egrep", "fgrep", "rg", "ag",
		"awk", "sed", "cut", "tr", "sort", "uniq", "tee",
		"paste", "join", "comm", "fold", "fmt", "column",
		"jq", "yq", "xq",
		// Search / navigation
		"which", "whereis", "type", "pwd", "cd", "tree", "realpath",
		"dirname", "basename",
		// Diff / compare
		"diff", "colordiff", "vimdiff", "cmp",
		// Environment
		"date", "cal", "uname", "hostname", "whoami", "id", "groups",
		"uptime", "free", "top", "htop", "ps", "pgrep", "lsof", "lsblk",
		"mount",
		// Help/docs
		"man", "info", "tldr", "help",
		// Linters / formatters
		"eslint", "prettier", "black", "ruff", "mypy", "pylint", "flake8",
		"gofmt", "golint", "rustfmt", "goimports", "pytest",
	}

	for _, cmd := range safeCommands {
		if !IsUnconditionalSafe(cmd) {
			t.Errorf("IsUnconditionalSafe(%q) = false, want true", cmd)
		}
	}

	// Non-safe commands should return false.
	unsafeCommands := []string{
		"rm", "mv", "cp", "chmod", "chown", "kill", "shutdown",
		"reboot", "mkfs", "dd", "curl", "wget", "ssh", "scp",
		"docker", "kubectl", "terraform", "git", "npm", "pip",
	}

	for _, cmd := range unsafeCommands {
		if IsUnconditionalSafe(cmd) {
			t.Errorf("IsUnconditionalSafe(%q) = true, want false", cmd)
		}
	}
}

func TestUnconditionalSafeCmd_MultiWord(t *testing.T) {
	tests := []struct {
		name     string
		cmd      string
		wantSafe bool
	}{
		// Multi-word safe commands from spec.
		{"cargo check", "cargo check", true},
		{"cargo test with args", "cargo test -- --nocapture", true},
		{"cargo clippy", "cargo clippy", true},
		{"cargo fmt", "cargo fmt", true},
		{"go vet", "go vet ./...", true},
		{"go test", "go test ./internal/core/", true},
		{"go fmt", "go fmt ./...", true},
		{"npm test", "npm test", true},
		{"npm run lint", "npm run lint", true},
		{"npm run test", "npm run test", true},
		{"npx jest", "npx jest --coverage", true},
		{"yarn test", "yarn test", true},
		{"pnpm test", "pnpm test", true},
		{"bun test", "bun test", true},
		{"python -m pytest", "python -m pytest tests/", true},
		{"python -m unittest", "python -m unittest discover", true},
		{"tsc --noEmit", "tsc --noEmit", true},
		{"tsc --version", "tsc --version", true},
		{"make check", "make check", true},
		{"make test", "make test", true},
		{"make lint", "make lint", true},
		{"node --version", "node --version", true},
		{"python --version", "python --version", true},
		{"go version", "go version", true},
		{"rustc --version", "rustc --version", true},
		{"cargo --version", "cargo --version", true},
		{"npm --version", "npm --version", true},
		{"git --version", "git --version", true},
		{"terraform --version", "terraform --version", true},
		{"aws --version", "aws --version", true},
		{"gcloud --version", "gcloud --version", true},
		{"az --version", "az --version", true},

		// Not safe multi-word commands.
		{"npm install", "npm install express", false},
		{"cargo build", "cargo build --release", false},
		{"go build", "go build ./cmd/fuse", false},
		{"make deploy", "make deploy", false},
		{"pip install", "pip install requests", false},

		// Single-word safe via basename lookup.
		{"plain ls", "ls -la", true},
		{"plain grep", "grep -r pattern .", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsUnconditionalSafeCmd(tt.cmd)
			if got != tt.wantSafe {
				t.Errorf("IsUnconditionalSafeCmd(%q) = %v, want %v", tt.cmd, got, tt.wantSafe)
			}
		})
	}
}

func TestConditionallySafe_Git(t *testing.T) {
	tests := []struct {
		name     string
		cmd      string
		wantSafe bool
	}{
		// Safe git subcommands.
		{"git status", "git status", true},
		{"git log", "git log --oneline -20", true},
		{"git diff", "git diff HEAD~1", true},
		{"git show", "git show HEAD", true},
		{"git branch list", "git branch", true},
		{"git branch -a", "git branch -a", true},
		{"git stash list", "git stash list", true},
		{"git remote", "git remote", true},
		{"git remote -v", "git remote -v", true},
		{"git remote show", "git remote show origin", true},
		{"git fetch", "git fetch origin", true},
		{"git pull", "git pull", true},
		{"git checkout -b", "git checkout -b new-feature", true},
		{"git config --list", "git config --list", true},
		{"git config --get", "git config --get user.name", true},
		{"git rev-parse", "git rev-parse HEAD", true},
		{"git describe", "git describe --tags", true},
		{"git tag list", "git tag -l", true},
		{"git tag plain", "git tag", true},
		{"git shortlog", "git shortlog -sn", true},
		{"git ls-files", "git ls-files", true},
		{"git ls-tree", "git ls-tree HEAD", true},
		{"git with -C flag", "git -C /tmp status", true},

		// Unsafe git subcommands.
		{"git push", "git push origin main", false},
		{"git push --force", "git push --force origin main", false},
		{"git reset --hard", "git reset --hard HEAD~1", false},
		{"git clean", "git clean -fd", false},
		{"git checkout -- .", "git checkout -- .", false},
		{"git checkout no -b", "git checkout main", false},
		{"git branch -D", "git branch -D feature", false},
		{"git branch -d", "git branch -d feature", false},
		{"git branch --delete", "git branch --delete feature", false},
		{"git stash drop", "git stash drop", false},
		{"git stash pop", "git stash pop", false},
		{"git pull --force", "git pull --force", false},
		{"git merge", "git merge feature", false},
		{"git rebase", "git rebase main", false},
		{"git commit", "git commit -m 'message'", false},
		{"git add", "git add .", false},
		{"git remote add", "git remote add upstream url", false},
		{"git config set", "git config user.name foo", false},
		{"git tag create", "git tag -a v1.0", false},
		{"bare git", "git", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsConditionallySafe("git", tt.cmd)
			if got != tt.wantSafe {
				t.Errorf("IsConditionallySafe(git, %q) = %v, want %v", tt.cmd, got, tt.wantSafe)
			}
		})
	}
}

func TestGitRestoreSafe_WorktreeWins(t *testing.T) {
	if gitRestoreSafe([]string{"--staged", "--worktree"}) {
		t.Fatal("expected git restore --staged --worktree to be unsafe")
	}
	if gitRestoreSafe([]string{"-SW"}) {
		t.Fatal("expected bundled -SW flags to be unsafe")
	}
	if !gitRestoreSafe([]string{"--staged"}) {
		t.Fatal("expected git restore --staged to be safe")
	}
}

func TestIsSqliteSafe_NormalizesQuotedKeywords(t *testing.T) {
	if isSqliteSafe([]string{"sqlite3", "db.sqlite", `"DR""OP"`, `"TABLE"`, `"users"`}) {
		t.Fatal("expected quoted DROP fragments to be unsafe")
	}
}

func TestIsSqliteSafe_SafeDotCommands(t *testing.T) {
	safe := [][]string{
		{"sqlite3", "db.sqlite", ".tables"},
		{"sqlite3", "db.sqlite", ".schema"},
		{"sqlite3", "db.sqlite", ".headers", "on"},
		{"sqlite3", "db.sqlite", ".mode", "csv"},
		{"sqlite3", "db.sqlite", ".databases"},
		{"sqlite3", "db.sqlite", ".indices"},
		{"sqlite3", "db.sqlite", ".dbinfo"},
		{"sqlite3", "db.sqlite", ".fullschema"},
	}
	for _, fields := range safe {
		if !isSqliteSafe(fields) {
			t.Errorf("expected %v to be safe", fields)
		}
	}
}

func TestIsSqliteSafe_DangerousDotCommands(t *testing.T) {
	dangerous := [][]string{
		{"sqlite3", "db.sqlite", ".shell", "rm", "-rf", "/"},
		{"sqlite3", "db.sqlite", ".system", "curl", "http://evil.com"},
		{"sqlite3", "db.sqlite", ".output", "/tmp/dump.sql"},
		{"sqlite3", "db.sqlite", ".import", "malicious.csv", "users"},
		{"sqlite3", "db.sqlite", ".load", "/tmp/evil.so"},
		{"sqlite3", "db.sqlite", ".read", "/tmp/evil.sql"},
		{"sqlite3", "db.sqlite", ".save", "/tmp/copy.db"},
		{"sqlite3", "db.sqlite", ".restore", "main", "/tmp/evil.db"},
		{"sqlite3", "db.sqlite", ".clone", "/tmp/copy.db"},
		{"sqlite3", "db.sqlite", ".unknown_command"},
	}
	for _, fields := range dangerous {
		if isSqliteSafe(fields) {
			t.Errorf("expected %v to be unsafe", fields)
		}
	}
}

func TestIsNcSafe_RejectsExecFlags(t *testing.T) {
	if isNcSafe([]string{"nc", "-ze", "example.com", "80"}) {
		t.Fatal("expected combined exec flags to make nc unsafe")
	}
	if isNcSafe([]string{"nc", "--exec=/bin/sh", "-z", "example.com", "80"}) {
		t.Fatal("expected long exec flags to make nc unsafe")
	}
	if !isNcSafe([]string{"nc", "-zv", "example.com", "80"}) {
		t.Fatal("expected scan-only nc flags to remain safe")
	}
}

func TestConditionallySafe_Terraform(t *testing.T) {
	tests := []struct {
		name     string
		cmd      string
		wantSafe bool
	}{
		{"terraform plan", "terraform plan", true},
		{"terraform validate", "terraform validate", true},
		{"terraform fmt", "terraform fmt", true},
		{"terraform show", "terraform show", true},
		{"terraform output", "terraform output", true},
		{"terraform providers", "terraform providers", true},
		{"terraform version", "terraform version", true},
		{"terraform graph", "terraform graph", true},
		{"tofu plan", "tofu plan", true},
		{"tofu validate", "tofu validate", true},

		{"terraform apply", "terraform apply", false},
		{"terraform destroy", "terraform destroy", false},
		{"terraform taint", "terraform taint resource", false},
		{"terraform state rm", "terraform state rm resource", false},
		{"terraform init", "terraform init", false},
		{"tofu apply", "tofu apply", false},
		{"bare terraform", "terraform", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			basename := "terraform"
			if len(tt.cmd) >= 4 && tt.cmd[:4] == "tofu" {
				basename = "tofu"
			}
			got := IsConditionallySafe(basename, tt.cmd)
			if got != tt.wantSafe {
				t.Errorf("IsConditionallySafe(%s, %q) = %v, want %v", basename, tt.cmd, got, tt.wantSafe)
			}
		})
	}
}

func TestConditionallySafe_Kubectl(t *testing.T) {
	tests := []struct {
		name     string
		cmd      string
		wantSafe bool
	}{
		{"kubectl get pods", "kubectl get pods", true},
		{"kubectl get all", "kubectl get all -n default", true},
		{"kubectl describe pod", "kubectl describe pod my-pod", true},
		{"kubectl logs", "kubectl logs my-pod", true},
		{"kubectl top", "kubectl top pods", true},
		{"kubectl version", "kubectl version", true},
		{"kubectl config view", "kubectl config view", true},
		{"kubectl api-resources", "kubectl api-resources", true},
		{"kubectl cluster-info", "kubectl cluster-info", true},
		{"kubectl explain", "kubectl explain pods", true},
		{"kubectl api-versions", "kubectl api-versions", true},

		{"kubectl delete", "kubectl delete pod my-pod", false},
		{"kubectl apply", "kubectl apply -f manifest.yaml", false},
		{"kubectl create", "kubectl create deployment nginx", false},
		{"kubectl drain", "kubectl drain node-1", false},
		{"kubectl replace --force", "kubectl replace --force -f pod.yaml", false},
		{"kubectl exec", "kubectl exec -it my-pod -- bash", false},
		{"kubectl config set", "kubectl config set-context my-ctx", false},
		{"bare kubectl", "kubectl", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsConditionallySafe("kubectl", tt.cmd)
			if got != tt.wantSafe {
				t.Errorf("IsConditionallySafe(kubectl, %q) = %v, want %v", tt.cmd, got, tt.wantSafe)
			}
		})
	}
}

func TestConditionallySafe_Docker(t *testing.T) {
	tests := []struct {
		name     string
		cmd      string
		wantSafe bool
	}{
		{"docker ps", "docker ps", true},
		{"docker ps -a", "docker ps -a", true},
		{"docker images", "docker images", true},
		{"docker logs", "docker logs my-container", true},
		{"docker inspect", "docker inspect my-container", true},
		{"docker stats", "docker stats", true},
		{"docker top", "docker top my-container", true},
		{"docker version", "docker version", true},
		{"docker info", "docker info", true},
		{"docker network ls", "docker network ls", true},
		{"docker volume ls", "docker volume ls", true},

		{"docker run", "docker run nginx", false},
		{"docker rm", "docker rm my-container", false},
		{"docker rmi", "docker rmi nginx:latest", false},
		{"docker system prune", "docker system prune", false},
		{"docker run --privileged", "docker run --privileged nginx", false},
		{"docker exec", "docker exec -it my-container bash", false},
		{"docker build", "docker build .", false},
		{"docker network create", "docker network create my-net", false},
		{"docker volume rm", "docker volume rm my-vol", false},
		{"bare docker", "docker", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsConditionallySafe("docker", tt.cmd)
			if got != tt.wantSafe {
				t.Errorf("IsConditionallySafe(docker, %q) = %v, want %v", tt.cmd, got, tt.wantSafe)
			}
		})
	}
}

func TestConditionallySafe_Aws(t *testing.T) {
	tests := []struct {
		name     string
		cmd      string
		wantSafe bool
	}{
		{"aws s3 ls", "aws s3 ls", true},
		{"aws s3 ls bucket", "aws s3 ls s3://my-bucket", true},
		{"aws ec2 describe-instances", "aws ec2 describe-instances", true},
		{"aws ec2 describe-vpcs", "aws ec2 describe-vpcs", true},
		{"aws iam list-users", "aws iam list-users", true},
		{"aws sts get-caller-identity", "aws sts get-caller-identity", true},
		{"aws s3api list-buckets", "aws s3api list-buckets", true},
		{"aws lambda get-function", "aws lambda get-function --function-name my-func", true},
		{"aws with region flag", "aws --region us-east-1 ec2 describe-instances", true},

		{"aws s3 rm", "aws s3 rm s3://my-bucket/file", false},
		{"aws s3 cp", "aws s3 cp file s3://bucket/", false},
		{"aws ec2 terminate-instances", "aws ec2 terminate-instances --instance-ids i-123", false},
		{"aws ec2 create-instance", "aws ec2 create-instance", false},
		{"aws iam delete-user", "aws iam delete-user --user-name foo", false},
		{"aws lambda invoke", "aws lambda invoke --function-name my-func out.json", false},
		{"bare aws", "aws", false},
		{"aws single token", "aws s3", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsConditionallySafe("aws", tt.cmd)
			if got != tt.wantSafe {
				t.Errorf("IsConditionallySafe(aws, %q) = %v, want %v", tt.cmd, got, tt.wantSafe)
			}
		})
	}
}

func TestConditionallySafe_Find(t *testing.T) {
	tests := []struct {
		name     string
		cmd      string
		wantSafe bool
	}{
		{"find basic", "find . -name '*.go'", true},
		{"find with type", "find /usr -type f -name '*.conf'", true},
		{"find with -delete", "find . -name '*.tmp' -delete", false},
		{"find with -exec rm", "find . -exec rm {} ;", false},
		{"find with -exec sh", "find . -exec sh -c 'echo {}' ;", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsConditionallySafe("find", tt.cmd)
			if got != tt.wantSafe {
				t.Errorf("IsConditionallySafe(find, %q) = %v, want %v", tt.cmd, got, tt.wantSafe)
			}
		})
	}
}

func TestConditionallySafe_Sed(t *testing.T) {
	tests := []struct {
		name     string
		cmd      string
		wantSafe bool
	}{
		{"sed display", "sed -n 's/foo/bar/p' file.txt", true},
		{"sed with -i", "sed -i 's/foo/bar/g' file.txt", false},
		{"sed with --in-place", "sed --in-place 's/foo/bar/g' file.txt", false},
		{"sed with -i.bak", "sed -i.bak 's/foo/bar/g' file.txt", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsConditionallySafe("sed", tt.cmd)
			if got != tt.wantSafe {
				t.Errorf("IsConditionallySafe(sed, %q) = %v, want %v", tt.cmd, got, tt.wantSafe)
			}
		})
	}
}

func TestConditionallySafe_Gcloud(t *testing.T) {
	tests := []struct {
		name     string
		cmd      string
		wantSafe bool
	}{
		{"gcloud describe", "gcloud compute instances describe my-instance", true},
		{"gcloud list", "gcloud compute instances list", true},
		{"gcloud config list", "gcloud config list", true},
		{"gcloud info", "gcloud info", true},
		{"gcloud auth list", "gcloud auth list", true},

		{"gcloud delete", "gcloud compute instances delete my-instance", false},
		{"gcloud create", "gcloud compute instances create my-instance", false},
		{"gcloud update", "gcloud compute instances update my-instance", false},
		{"bare gcloud", "gcloud", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsConditionallySafe("gcloud", tt.cmd)
			if got != tt.wantSafe {
				t.Errorf("IsConditionallySafe(gcloud, %q) = %v, want %v", tt.cmd, got, tt.wantSafe)
			}
		})
	}
}

func TestConditionallySafe_Az(t *testing.T) {
	tests := []struct {
		name     string
		cmd      string
		wantSafe bool
	}{
		{"az show", "az vm show --name my-vm", true},
		{"az list", "az vm list", true},
		{"az account show", "az account show", true},

		{"az delete", "az vm delete --name my-vm", false},
		{"az create", "az vm create --name my-vm", false},
		{"az update", "az vm update --name my-vm", false},
		{"bare az", "az", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsConditionallySafe("az", tt.cmd)
			if got != tt.wantSafe {
				t.Errorf("IsConditionallySafe(az, %q) = %v, want %v", tt.cmd, got, tt.wantSafe)
			}
		})
	}
}

func TestConditionallySafe_Pulumi(t *testing.T) {
	tests := []struct {
		name     string
		cmd      string
		wantSafe bool
	}{
		{"pulumi preview", "pulumi preview", true},
		{"pulumi stack ls", "pulumi stack ls", true},
		{"pulumi config", "pulumi config", true},
		{"pulumi version", "pulumi version", true},
		{"pulumi about", "pulumi about", true},

		{"pulumi up", "pulumi up", false},
		{"pulumi destroy", "pulumi destroy", false},
		{"pulumi stack rm", "pulumi stack rm my-stack", false},
		{"bare pulumi", "pulumi", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsConditionallySafe("pulumi", tt.cmd)
			if got != tt.wantSafe {
				t.Errorf("IsConditionallySafe(pulumi, %q) = %v, want %v", tt.cmd, got, tt.wantSafe)
			}
		})
	}
}

func TestConditionallySafe_NegativeCases(t *testing.T) {
	// Commands not in the conditionally safe table should return false.
	tests := []struct {
		name     string
		basename string
		cmd      string
	}{
		{"rm", "rm", "rm -rf /"},
		{"curl", "curl", "curl http://example.com"},
		{"wget", "wget", "wget http://example.com"},
		{"chmod", "chmod", "chmod 777 file"},
		{"chown", "chown", "chown root file"},
		{"kill", "kill", "kill -9 1234"},
		{"ssh", "ssh", "ssh user@host"},
		{"pip", "pip", "pip install package"},
		{"npm", "npm", "npm install package"},
		{"make", "make", "make all"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsConditionallySafe(tt.basename, tt.cmd)
			if got {
				t.Errorf("IsConditionallySafe(%q, %q) = true, want false", tt.basename, tt.cmd)
			}
		})
	}
}

func TestConditionallySafe_Xargs(t *testing.T) {
	tests := []struct {
		name     string
		cmd      string
		wantSafe bool
	}{
		{"xargs echo", "xargs echo", true},
		{"xargs grep", "xargs grep pattern", true},
		{"xargs rm", "xargs rm", false},
		{"xargs kill", "xargs kill", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsConditionallySafe("xargs", tt.cmd)
			if got != tt.wantSafe {
				t.Errorf("IsConditionallySafe(xargs, %q) = %v, want %v", tt.cmd, got, tt.wantSafe)
			}
		})
	}
}

func TestConditionallySafe_Base64(t *testing.T) {
	tests := []struct {
		name     string
		cmd      string
		wantSafe bool
	}{
		{"base64 encode", "base64 file.txt", true},
		{"base64 decode -d", "base64 -d file.txt", false},
		{"base64 decode --decode", "base64 --decode file.txt", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsConditionallySafe("base64", tt.cmd)
			if got != tt.wantSafe {
				t.Errorf("IsConditionallySafe(base64, %q) = %v, want %v", tt.cmd, got, tt.wantSafe)
			}
		})
	}
}
