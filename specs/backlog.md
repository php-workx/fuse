# Backlog

## Per-tag enforcement mode

**Priority**: Medium
**Status**: Planned

The three-state mode (enabled/dryrun/disabled) is global. Users need per-tag control:

```yaml
# ~/.fuse/config/policy.yaml
tag_overrides:
  cloudformation: dryrun   # log but don't block
  git: enabled             # full enforcement
  payment: disabled        # don't even evaluate
```

| Override | Effect |
|----------|--------|
| `enabled` | Full enforcement — default |
| `dryrun` | Classify and log, but always allow |
| `disabled` | Skip evaluation entirely |

**Use cases**: gradual rollout of new rule categories, per-project tuning, debugging noisy categories.

**Implementation**: Add `TagOverrides map[string]string` to `PolicyConfig`. In `EvaluateBuiltins`, check tag overrides after `disabledTags`. Dryrun override evaluates and logs but returns empty (no enforcement).

---

## Port remaining DCG detection packs

**Priority**: Medium
**Status**: In progress (architecture done, rules pending)

50+ packs from DCG to port. Architecture (tags, keywords, progressive activation) is ready. Remaining categories:

- CI/CD (GitHub Actions, GitLab CI, Jenkins, CircleCI)
- CDN (Cloudflare Workers, CloudFront, Fastly)
- DNS (Cloudflare, Route53, generic)
- API Gateways (Apigee, AWS, Kong)
- Email services (SES, SendGrid, Mailgun, Postmark)
- Feature flags (LaunchDarkly, Split, Unleash, Flipt)
- Load balancers (ELB, nginx, HAProxy, Traefik)
- Messaging (Kafka, RabbitMQ, NATS, SQS)
- Monitoring (Datadog, Prometheus, NewRelic, PagerDuty, Splunk)
- Payment (Stripe, Braintree, Square)
- Search engines (Elasticsearch, OpenSearch, Algolia, Meilisearch)
- Backups (restic, borg, rclone, velero)
- Secrets management (Vault, AWS Secrets, Doppler, 1Password)
- Platform (GitHub, GitLab)
- Container escape detection (docker socket, nsenter, cgroup)

See `specs/rule-landscape.md` for the full gap analysis.

---

## Recursive command unwrapping

**Priority**: Medium
**Status**: Planned

Port AgentGuard's recursive unwrapping: catch destructive commands hidden inside `sudo bash -c`, `xargs`, `find -exec`, `chroot`, etc. Current fuse only handles sudo/doas stripping in normalize.go.

---

## Anti-evasion preprocessing

**Priority**: Low
**Status**: Planned

Port from Rubberband:
- Unicode NFKC normalization (homoglyph attacks)
- URL decoding (%2F-style obfuscation)
- Shell escape expansion ($'...' syntax)
- Path collapsing (../../ traversal chains)
- 10,000-char command limit (ReDoS prevention)

---

## CLI events coverage gap

**Priority**: Low
**Status**: Planned

`internal/cli/events.go` has low test coverage. The `local-observability` branch has tests but they'll need updating as the CLI evolves. Not blocking — the DB query layer is well-tested.
