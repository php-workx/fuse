# Rule Landscape — Cross-Project Analysis

Categorized overview of detection rules across 7 projects.
Goal: identify what fuse covers, what's missing, and where to source rules from.

## Coverage Matrix

| Category | Fuse | DCG | AgentGuard | Rubberband | TaskPilot | OpenGuardrails |
|----------|:----:|:---:|:----------:|:----------:|:---------:|:--------------:|
| **Filesystem (rm -rf, shred, find -delete)** | 22 hardcoded + 6 builtin | Core pack | 13 block + 18 confirm + 52 allow | - | Yes | - |
| **Git (reset, clean, force-push)** | 9 builtin | 12 patterns | 6 confirm | - | - | - |
| **AWS CLI** | 46 builtin | Full pack | - | - | - | - |
| **GCP CLI** | 25 builtin | Full pack | - | - | - | - |
| **Azure CLI** | 20 builtin | Full pack | - | - | - | - |
| **Terraform/Tofu** | 9 builtin | Full pack | - | - | - | - |
| **Pulumi** | 7 builtin | Full pack | - | - | - | - |
| **Kubernetes** | 5 builtin | 3 packs (kubectl/helm/kustomize) | - | - | - | - |
| **Docker/Containers** | 7 builtin | 3 packs (docker/compose/podman) | - | Container escape | - | - |
| **Databases (SQL, Redis, Mongo)** | 11 builtin | 6 packs (pg/mysql/mongo/redis/sqlite/supabase) | - | - | - | - |
| **Inline scripts (python -c, bash -c)** | 15 patterns + safe python check | - | Script analyzer (30+ patterns) | - | - | - |
| **Reverse shells** | 5 builtin | - | - | 90 score | Yes | - |
| **Exfiltration** | 9 builtin | - | - | 40 score | Yes | Behavioral chain |
| **Credential access** | 9 builtin | 4 secrets packs | - | 60-70 score | Yes | S08 scanner |
| **Obfuscation** | 6 builtin | - | - | - | Yes | - |
| **Persistence** | 7 builtin | - | - | 60 score | - | - |
| **Package managers** | 8 builtin | 1 pack | - | - | - | - |
| **System services** | 7 builtin | 3 packs | - | - | Yes | - |
| **PaaS (Heroku, Fly, Railway)** | 5 builtin | - | - | - | - | - |
| **Recon (nmap, masscan)** | 4 builtin | - | - | - | Yes | - |
| **Self-protection** | 8 hardcoded | - | - | - | - | - |
| **CI/CD** | - | 4 packs (GH/GL/Jenkins/Circle) | - | - | - | - |
| **CDN** | - | 3 packs (CF Workers/CloudFront/Fastly) | - | - | - | - |
| **DNS** | - | 3 packs (CF/Route53/generic) | - | - | - | - |
| **API Gateways** | - | 3 packs (Apigee/AWS/Kong) | - | - | - | - |
| **Email services** | - | 4 packs (SES/SendGrid/Mailgun/Postmark) | - | - | - | - |
| **Feature flags** | - | 4 packs (LaunchDarkly/Split/Unleash/Flipt) | - | - | - | - |
| **Load balancers** | - | 4 packs (ELB/nginx/HAProxy/Traefik) | - | - | - | - |
| **Messaging** | - | 4 packs (Kafka/RabbitMQ/NATS/SQS) | - | - | - | - |
| **Monitoring** | - | 5 packs (Datadog/Prometheus/NewRelic/PD/Splunk) | - | - | - | - |
| **Payment** | - | 3 packs (Stripe/Braintree/Square) | - | - | - | - |
| **Search engines** | - | 4 packs (ES/OpenSearch/Algolia/Meilisearch) | - | - | - | - |
| **Backups** | - | 4 packs (restic/borg/rclone/velero) | - | - | - | - |
| **Recursive unwrapping** | mvdan.cc/sh compound split | - | Full (sudo/bash -c/xargs/find -exec) | - | - | - |
| **Safe allowlist (build dirs)** | - | temp dirs only | 52 patterns (node_modules, dist, build, .next, target, etc.) | - | - | - |
| **Anti-evasion preprocessing** | ANSI strip, whitespace normalize | - | Env var expansion, quote handling | Unicode NFKC, URL decode, shell escape, path collapse | - | - |
| **Container escape** | - | - | - | Yes (docker socket, nsenter, cgroup) | - | - |
| **Windows threats** | - | - | - | Yes (mimikatz, PowerShell, SAM) | - | - |
| **Prompt injection** | - | - | - | - | InputSanitizer | S01 scanner |
| **PII scrubbing** | Credential scrub (5 patterns) | - | - | - | 20+ patterns | Reversible masking |
| **Behavioral chains** | - | - | - | - | - | Multi-step detection |
| **Intent mismatch** | - | - | - | - | - | Yes |
| **MCP tool poisoning** | - | - | - | - | - | S04 scanner |
| **Numeric risk scoring** | - | - | Specificity scoring | 0-100 scale | - | 0.0-1.0 continuous |

## Key Gaps in Fuse

### Priority 1 — High-value, low-effort (adopt patterns from other projects)

| Gap | Source | Effort |
|-----|--------|--------|
| **Safe build dir allowlist** | AgentGuard: 52 patterns for node_modules, dist, build, .next, target, __pycache__, .cache, coverage, tmp | Small — add to builtins |
| **CI/CD protection** | DCG: 4 packs (GH Actions, GitLab CI, Jenkins, CircleCI secrets/variables) | Medium — port regex patterns |
| **Container escape detection** | Rubberband: docker socket mount, nsenter, cgroup escape | Small — add to builtins |
| **Recursive unwrapping** | AgentGuard: sudo/bash -c/xargs/find -exec chain | Medium — enhance normalize.go |

### Priority 2 — Medium-value, medium-effort

| Gap | Source | Effort |
|-----|--------|--------|
| **CDN/DNS/API gateway** | DCG: 9 packs | Medium — port patterns |
| **Messaging (Kafka, RabbitMQ)** | DCG: 4 packs | Medium — port patterns |
| **Secrets management (Vault, 1Password)** | DCG: 4 packs | Medium — port patterns |
| **Search engines (ES, Algolia)** | DCG: 4 packs | Medium — port patterns |
| **Backup tools (restic, borg, rclone)** | DCG: 4 packs | Small — port patterns |
| **Anti-evasion: Unicode NFKC** | Rubberband | Small — add to normalize.go |
| **Risk scoring** | Rubberband: 0-100 numeric | Medium — new scoring model |

### Priority 3 — High-value but significant architectural work

| Gap | Source | Effort |
|-----|--------|--------|
| **Behavioral chain detection** | OpenGuardrails | Large — needs multi-event correlation |
| **Prompt injection defense** | TaskPilot/OpenGuardrails | Large — content analysis layer |
| **PII scrubbing (20+ patterns)** | TaskPilot | Medium — extend credential scrub |
| **Reversible data masking** | OpenGuardrails | Large — needs round-trip architecture |
| **LLM-powered command explanation** | TaskPilot | Medium — optional LLM integration |

### Not applicable for v1

| Gap | Reason to skip |
|-----|----------------|
| Windows threats (mimikatz, PowerShell, SAM) | fuse is Unix-only (no Windows TTY) |
| NSFW/off-topic detection | Out of scope (command safety, not content moderation) |
| Model switching | fuse doesn't control the LLM |

## Unique Patterns to Port (not in fuse today)

### From AgentGuard — Safe build dir allowlist
```
rm -rf node_modules, dist, build, .next, out, target, .cache,
__pycache__, .pytest_cache, tmp, temp, coverage, .nyc_output
```
These should be SAFE (or at least not BLOCKED) even with `-rf` flags.

### From AgentGuard — Wrapper unwrapping
```
sudo, doas, env, nice, nohup, time, timeout, watch, strace, ltrace,
ionice, chroot, runuser, xargs, parallel, find -exec, find -delete
```
fuse already strips sudo/doas in normalize.go but doesn't handle xargs, find -exec, or chroot.

### From DCG — Database-specific patterns not in fuse
```
DROP SCHEMA, RESET MASTER, GRANT ALL ON *.*,
pg_dump --clean, mongorestore --drop, mysqldump --add-drop-database,
Redis CONFIG SET dir/dbfilename (attack vector), DEBUG SEGFAULT
```

### From DCG — CI/CD patterns (new category for fuse)
```
gh api DELETE .../actions/secrets/..., gh secret delete,
gitlab-runner unregister, jenkins delete-job,
circleci context remove-secret
```

### From Rubberband — Anti-evasion preprocessing
```
Unicode NFKC normalization (homoglyph attacks)
URL decoding (%2F-style obfuscation)
Shell escape expansion ($'...' syntax)
Path collapsing (../../ traversal chains)
10,000-char command limit (ReDoS prevention)
```

### From Rubberband — Container escape patterns
```
-v /var/run/docker.sock, nsenter --target 1,
--privileged + --pid=host, cgroup escape patterns
```
