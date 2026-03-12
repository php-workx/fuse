package policy

import (
	"regexp"
	"strings"

	"github.com/runger/fuse/internal/core"
)

var reNetcatScanMode = regexp.MustCompile(`(^|\s)-[a-zA-Z]*z[a-zA-Z]*(\s|$)`)

func init() {
	BuiltinRules = append(BuiltinRules, []BuiltinRule{
		// ---------------------------------------------------------------
		// §6.3.12 PaaS CLIs
		// ---------------------------------------------------------------
		{
			ID:      "builtin:paas:heroku-destroy",
			Pattern: regexp.MustCompile(`\bheroku\s+apps:destroy\b`),
			Action:  core.DecisionApproval,
			Reason:  "Destroys Heroku app",
		},
		{
			ID:      "builtin:paas:fly-destroy",
			Pattern: regexp.MustCompile(`\bfly(ctl)?\s+destroy\b`),
			Action:  core.DecisionApproval,
			Reason:  "Destroys Fly.io app",
		},
		{
			ID:      "builtin:paas:vercel-rm",
			Pattern: regexp.MustCompile(`\bvercel\s+rm\b`),
			Action:  core.DecisionApproval,
			Reason:  "Deletes Vercel project",
		},
		{
			ID:      "builtin:paas:netlify-delete",
			Pattern: regexp.MustCompile(`\bnetlify\s+sites:delete\b`),
			Action:  core.DecisionApproval,
			Reason:  "Deletes Netlify site",
		},
		{
			ID:      "builtin:paas:railway-delete",
			Pattern: regexp.MustCompile(`\brailway\s+delete\b`),
			Action:  core.DecisionApproval,
			Reason:  "Deletes Railway project",
		},

		// ---------------------------------------------------------------
		// §6.3.13 Local filesystem
		// ---------------------------------------------------------------
		{
			ID:      "builtin:fs:rm-rf",
			Pattern: regexp.MustCompile(`\brm\s+(-[a-zA-Z]*r[a-zA-Z]*f|f[a-zA-Z]*r)\b`),
			Action:  core.DecisionApproval,
			Reason:  "Recursive force-remove (non-root paths)",
		},
		{
			ID:      "builtin:fs:rm-split-rf",
			Pattern: regexp.MustCompile(`\brm\s+.*-r\b.*-f\b`),
			Action:  core.DecisionApproval,
			Reason:  "rm with split -r -f flags",
		},
		{
			ID:      "builtin:fs:rm-long-rf",
			Pattern: regexp.MustCompile(`\brm\s+.*--recursive\b.*--force\b`),
			Action:  core.DecisionApproval,
			Reason:  "rm with long-form flags",
		},
		{
			ID:      "builtin:fs:find-delete",
			Pattern: regexp.MustCompile(`\bfind\b.*\s-delete\b`),
			Action:  core.DecisionApproval,
			Reason:  "Find with delete",
		},
		{
			ID:      "builtin:fs:find-exec-rm",
			Pattern: regexp.MustCompile(`\bfind\b.*-exec\s+rm\b`),
			Action:  core.DecisionApproval,
			Reason:  "Find with exec rm",
		},
		{
			ID:      "builtin:fs:shred",
			Pattern: regexp.MustCompile(`\bshred\b`),
			Action:  core.DecisionCaution,
			Reason:  "Secure file deletion",
		},

		// ---------------------------------------------------------------
		// §6.3.14 Suspicious interpreter launches
		// ---------------------------------------------------------------
		{
			ID:      "builtin:interp:python-file",
			Pattern: regexp.MustCompile(`\bpython[23]?\s+\S+\.py\b`),
			Action:  core.DecisionCaution,
			Reason:  "Python script execution",
		},
		{
			ID:      "builtin:interp:node-file",
			Pattern: regexp.MustCompile(`\bnode\s+\S+\.[jt]s\b`),
			Action:  core.DecisionCaution,
			Reason:  "Node script execution",
		},
		{
			ID:      "builtin:interp:bash-file",
			Pattern: regexp.MustCompile(`\b(ba)?sh\s+\S+\.sh\b`),
			Action:  core.DecisionCaution,
			Reason:  "Shell script execution",
		},

		// ---------------------------------------------------------------
		// §6.3.15 Credential access & secret exposure
		// ---------------------------------------------------------------
		{
			ID:      "builtin:cred:env-dump",
			Pattern: regexp.MustCompile(`\b(env|printenv|set)\b\s*$`),
			Action:  core.DecisionCaution,
			Reason:  "Dumps all environment variables (may contain secrets)",
		},
		{
			ID:      "builtin:cred:cat-credentials",
			Pattern: regexp.MustCompile(`\b(cat|less|more|head|tail)\s+.*\.(pem|key|crt|p12|pfx|jks|keystore)\b`),
			Action:  core.DecisionCaution,
			Reason:  "Reads credential/key files",
		},
		{
			ID:      "builtin:cred:cat-cloud-creds",
			Pattern: regexp.MustCompile(`\b(cat|less|more|head|tail)\s+.*(credentials|\.aws\/config|\.boto|\.gcloud|\.azure|service.account\.json|kubeconfig)\b`),
			Action:  core.DecisionApproval,
			Reason:  "Reads cloud credential files",
		},
		{
			ID:      "builtin:cred:ssh-key-read",
			Pattern: regexp.MustCompile(`\b(cat|less|more|head|tail)\s+.*\.ssh\/(id_|authorized_keys|known_hosts)`),
			Action:  core.DecisionCaution,
			Reason:  "Reads SSH key material",
		},
		{
			ID:      "builtin:cred:history-read",
			Pattern: regexp.MustCompile(`\b(cat|less|more|head|tail)\s+.*(\.bash_history|\.zsh_history|\.histfile)`),
			Action:  core.DecisionCaution,
			Reason:  "Reads shell history (may contain secrets)",
		},
		{
			ID:      "builtin:cred:docker-config",
			Pattern: regexp.MustCompile(`\b(cat|less|more|head|tail)\s+.*\.docker\/config\.json`),
			Action:  core.DecisionCaution,
			Reason:  "Reads Docker registry credentials",
		},
		{
			ID:      "builtin:cred:npm-token",
			Pattern: regexp.MustCompile(`\b(cat|less|more|head|tail)\s+.*\.npmrc\b`),
			Action:  core.DecisionCaution,
			Reason:  "Reads npm auth tokens",
		},
		{
			ID:      "builtin:cred:copy-creds",
			Pattern: regexp.MustCompile(`\bcp\s+.*\.(pem|key|crt|p12|pfx)\s+`),
			Action:  core.DecisionCaution,
			Reason:  "Copies credential files",
		},
		{
			ID:      "builtin:cred:base64-key",
			Pattern: regexp.MustCompile(`\bbase64\s+.*\.(pem|key|crt|p12)\b`),
			Action:  core.DecisionApproval,
			Reason:  "Base64-encodes credential files",
		},

		// ---------------------------------------------------------------
		// §6.3.16 Data exfiltration & staging
		// ---------------------------------------------------------------
		{
			ID:      "builtin:exfil:curl-post",
			Pattern: regexp.MustCompile(`\bcurl\s+.*(-X\s*POST|-d\s|--data)\b`),
			Action:  core.DecisionCaution,
			Reason:  "HTTP POST (potential data exfiltration)",
		},
		{
			ID:      "builtin:exfil:curl-upload",
			Pattern: regexp.MustCompile(`\bcurl\s+.*(-T|--upload-file|-F|--form)\b`),
			Action:  core.DecisionCaution,
			Reason:  "HTTP file upload",
		},
		{
			ID:      "builtin:exfil:wget-post",
			Pattern: regexp.MustCompile(`\bwget\s+.*--post-(data|file)\b`),
			Action:  core.DecisionCaution,
			Reason:  "wget POST (potential data exfiltration)",
		},
		{
			ID:      "builtin:exfil:tar-create",
			Pattern: regexp.MustCompile(`\btar\s+.*c[a-zA-Z]*f\s+.*\.(tar|gz|tgz|bz2|xz|zip)\b`),
			Action:  core.DecisionCaution,
			Reason:  "Creates archive (potential staging)",
		},
		{
			ID:      "builtin:exfil:zip-create",
			Pattern: regexp.MustCompile(`\bzip\s+(-r\s+)?.*\.(zip)\b`),
			Action:  core.DecisionCaution,
			Reason:  "Creates zip archive",
		},
		{
			ID:      "builtin:exfil:nc-connect",
			Pattern: regexp.MustCompile(`\b(nc|ncat|netcat)\s+.*\d+\.\d+\.\d+\.\d+`),
			Action:  core.DecisionApproval,
			Reason:  "Netcat connection to IP (potential exfiltration)",
			Predicate: func(cmd string) bool {
				return !reNetcatScanMode.MatchString(cmd)
			},
		},
		{
			ID:      "builtin:exfil:scp-out",
			Pattern: regexp.MustCompile(`\bscp\s+[^:]+\s+\S+:`),
			Action:  core.DecisionCaution,
			Reason:  "SCP copy to remote host",
		},
		{
			ID:      "builtin:exfil:dns-exfil",
			Pattern: regexp.MustCompile(`\b(dig|nslookup|host)\s+.*\$\(`),
			Action:  core.DecisionApproval,
			Reason:  "DNS lookup with command substitution (DNS exfiltration)",
		},
		{
			ID:      "builtin:exfil:redirect-tcp",
			Pattern: regexp.MustCompile(`>\s*/dev/tcp/`),
			Action:  core.DecisionApproval,
			Reason:  "Redirect to /dev/tcp (network exfiltration)",
		},

		// ---------------------------------------------------------------
		// §6.3.17 Reverse shells & persistence
		// ---------------------------------------------------------------
		{
			ID:      "builtin:revshell:bash-tcp",
			Pattern: regexp.MustCompile(`\bbash\s+.*-i\s+.*>/dev/tcp/`),
			Action:  core.DecisionApproval,
			Reason:  "Bash reverse shell via /dev/tcp",
		},
		{
			ID:      "builtin:revshell:python",
			Pattern: regexp.MustCompile(`\bpython[23]?\s+.*socket\..*connect\b`),
			Action:  core.DecisionApproval,
			Reason:  "Python reverse shell",
		},
		{
			ID:      "builtin:revshell:nc-exec",
			Pattern: regexp.MustCompile(`\b(nc|ncat|netcat)\s+.*-e\s+`),
			Action:  core.DecisionApproval,
			Reason:  "Netcat with exec (reverse shell)",
		},
		{
			ID:      "builtin:revshell:mkfifo",
			Pattern: regexp.MustCompile(`\bmkfifo\s+.*\b(nc|ncat|netcat)\b`),
			Action:  core.DecisionApproval,
			Reason:  "Named pipe reverse shell",
		},
		{
			ID:      "builtin:revshell:socat",
			Pattern: regexp.MustCompile(`\bsocat\s+.*TCP`),
			Action:  core.DecisionCaution,
			Reason:  "Socat TCP connection",
		},
		{
			ID:      "builtin:persist:crontab-edit",
			Pattern: regexp.MustCompile(`\bcrontab\s+(-e|-r|-l)\b`),
			Action:  core.DecisionCaution,
			Reason:  "Modifies crontab",
		},
		{
			ID:      "builtin:persist:cron-write",
			Pattern: regexp.MustCompile(`(>|>>)\s*.*(/etc/cron|/var/spool/cron)`),
			Action:  core.DecisionApproval,
			Reason:  "Writes to cron directories",
		},
		{
			ID:      "builtin:persist:systemd-enable",
			Pattern: regexp.MustCompile(`\bsystemctl\s+enable\b`),
			Action:  core.DecisionCaution,
			Reason:  "Enables systemd service (persistence)",
		},
		{
			ID:      "builtin:persist:launchd-load",
			Pattern: regexp.MustCompile(`\blaunchctl\s+(load|bootstrap)\b`),
			Action:  core.DecisionCaution,
			Reason:  "Loads macOS launch daemon/agent",
		},
		{
			ID:      "builtin:persist:profile-write",
			Pattern: regexp.MustCompile(`(>|>>)\s*.*(/etc/profile|/etc/bashrc|/etc/zshrc)`),
			Action:  core.DecisionApproval,
			Reason:  "Writes to system-wide shell profiles",
		},
		{
			ID:      "builtin:persist:sudoers-write",
			Pattern: regexp.MustCompile(`(>|>>|tee\s+(-a\s+)?|visudo).*(/etc/sudoers|/etc/sudoers\.d/)`),
			Action:  core.DecisionApproval,
			Reason:  "Modifies sudoers configuration",
		},
		{
			ID:      "builtin:persist:authorized-keys",
			Pattern: regexp.MustCompile(`(>|>>)\s*.*\.ssh/authorized_keys`),
			Action:  core.DecisionApproval,
			Reason:  "Writes to SSH authorized_keys",
		},

		// ---------------------------------------------------------------
		// §6.3.18 Container escape & privilege escalation
		// ---------------------------------------------------------------
		{
			ID:      "builtin:container:privileged",
			Pattern: regexp.MustCompile(`\bdocker\s+run\s+.*--privileged\b`),
			Action:  core.DecisionApproval,
			Reason:  "Runs privileged container (host access)",
		},
		{
			ID:      "builtin:container:host-pid",
			Pattern: regexp.MustCompile(`\bdocker\s+run\s+.*--pid=host\b`),
			Action:  core.DecisionApproval,
			Reason:  "Container with host PID namespace",
		},
		{
			ID:      "builtin:container:host-net",
			Pattern: regexp.MustCompile(`\bdocker\s+run\s+.*--network=host\b`),
			Action:  core.DecisionCaution,
			Reason:  "Container with host network",
		},
		{
			ID:      "builtin:container:mount-sock",
			Pattern: regexp.MustCompile(`\bdocker\s+run\s+.*-v\s+/var/run/docker\.sock`),
			Action:  core.DecisionApproval,
			Reason:  "Mounts Docker socket (container escape)",
		},
		{
			ID:      "builtin:container:mount-root",
			Pattern: regexp.MustCompile(`\bdocker\s+run\s+.*-v\s+/:/`),
			Action:  core.DecisionApproval,
			Reason:  "Mounts host root filesystem",
		},
		{
			ID:      "builtin:container:nsenter",
			Pattern: regexp.MustCompile(`\bnsenter\b`),
			Action:  core.DecisionApproval,
			Reason:  "Enters namespace (container escape)",
		},
		{
			ID:      "builtin:container:unshare",
			Pattern: regexp.MustCompile(`\bunshare\b`),
			Action:  core.DecisionCaution,
			Reason:  "Creates new namespace",
		},
		{
			ID:      "builtin:privesc:setuid",
			Pattern: regexp.MustCompile(`\bchmod\s+[0-7]*[4-7][0-7]{2}\s`),
			Action:  core.DecisionCaution,
			Reason:  "Sets setuid/setgid bits",
		},
		{
			ID:      "builtin:privesc:cap-add",
			Pattern: regexp.MustCompile(`\bdocker\s+run\s+.*--cap-add\s+(ALL|SYS_ADMIN|SYS_PTRACE)`),
			Action:  core.DecisionApproval,
			Reason:  "Adds dangerous Linux capabilities",
		},

		// ---------------------------------------------------------------
		// §6.3.19 Obfuscation & indirect execution
		// ---------------------------------------------------------------
		{
			ID:      "builtin:obfusc:base64-exec",
			Pattern: regexp.MustCompile(`\bbase64\s+(-d|--decode).*\|\s*(ba)?sh\b`),
			Action:  core.DecisionApproval,
			Reason:  "Base64 decode piped to shell",
		},
		{
			ID:      "builtin:obfusc:xxd-exec",
			Pattern: regexp.MustCompile(`\bxxd\s+.*-r.*\|\s*(ba)?sh\b`),
			Action:  core.DecisionApproval,
			Reason:  "Hex decode piped to shell",
		},
		{
			ID:      "builtin:obfusc:printf-exec",
			Pattern: regexp.MustCompile(`\bprintf\s+.*\\\\x.*\|\s*(ba)?sh\b`),
			Action:  core.DecisionApproval,
			Reason:  "Printf hex escape piped to shell",
		},
		{
			ID:      "builtin:obfusc:rev-exec",
			Pattern: regexp.MustCompile(`\brev\b.*\|\s*(ba)?sh\b`),
			Action:  core.DecisionApproval,
			Reason:  "String reversal piped to shell",
		},
		{
			ID:      "builtin:obfusc:curl-exec",
			Pattern: regexp.MustCompile(`\bcurl\s+.*\|\s*(ba)?sh\b`),
			Action:  core.DecisionApproval,
			Reason:  "curl piped to shell",
		},
		{
			ID:      "builtin:obfusc:wget-exec",
			Pattern: regexp.MustCompile(`\bwget\s+.*-O\s*-.*\|\s*(ba)?sh\b`),
			Action:  core.DecisionApproval,
			Reason:  "wget piped to shell",
		},
		{
			ID:      "builtin:indirect:xargs-exec",
			Pattern: regexp.MustCompile(`\bxargs\s+.*\b(rm|kill|chmod|chown)\b`),
			Action:  core.DecisionCaution,
			Reason:  "xargs with destructive command",
		},
		{
			ID:      "builtin:indirect:find-exec",
			Pattern: regexp.MustCompile(`\bfind\b.*-exec\s+(sh|bash)\s+-c\b`),
			Action:  core.DecisionApproval,
			Reason:  "find -exec with shell",
		},

		// ---------------------------------------------------------------
		// §6.3.20 Package managers
		// ---------------------------------------------------------------
		{
			ID:      "builtin:pkg:npm-global",
			Pattern: regexp.MustCompile(`\bnpm\s+install\s+.*-g\b`),
			Action:  core.DecisionCaution,
			Reason:  "Global npm package install",
		},
		{
			ID:      "builtin:pkg:pip-install",
			Pattern: regexp.MustCompile(`\bpip[3]?\s+install\b`),
			Action:  core.DecisionCaution,
			Reason:  "pip package install",
			Predicate: func(cmd string) bool {
				return !strings.Contains(cmd, "http://") && !strings.Contains(cmd, "https://")
			},
		},
		{
			ID:      "builtin:pkg:pip-install-url",
			Pattern: regexp.MustCompile(`\bpip[3]?\s+install\s+.*https?://`),
			Action:  core.DecisionApproval,
			Reason:  "pip install from URL",
		},
		{
			ID:      "builtin:pkg:gem-install",
			Pattern: regexp.MustCompile(`\bgem\s+install\b`),
			Action:  core.DecisionCaution,
			Reason:  "Ruby gem install",
		},
		{
			ID:      "builtin:pkg:cargo-install",
			Pattern: regexp.MustCompile(`\bcargo\s+install\b`),
			Action:  core.DecisionCaution,
			Reason:  "Cargo package install",
		},
		{
			ID:      "builtin:pkg:go-install",
			Pattern: regexp.MustCompile(`\bgo\s+install\b`),
			Action:  core.DecisionCaution,
			Reason:  "Go package install",
		},
		{
			ID:      "builtin:pkg:brew-uninstall",
			Pattern: regexp.MustCompile(`\bbrew\s+(uninstall|remove)\b`),
			Action:  core.DecisionCaution,
			Reason:  "Homebrew package removal",
		},
		{
			ID:      "builtin:pkg:apt-remove",
			Pattern: regexp.MustCompile(`\b(apt|apt-get)\s+(remove|purge|autoremove)\b`),
			Action:  core.DecisionCaution,
			Reason:  "APT package removal",
		},

		// ---------------------------------------------------------------
		// §6.3.21 Reconnaissance
		// ---------------------------------------------------------------
		{
			ID:      "builtin:recon:nmap",
			Pattern: regexp.MustCompile(`\bnmap\b`),
			Action:  core.DecisionCaution,
			Reason:  "Network port scanning",
		},
		{
			ID:      "builtin:recon:masscan",
			Pattern: regexp.MustCompile(`\bmasscan\b`),
			Action:  core.DecisionApproval,
			Reason:  "Aggressive network scanning",
		},
		{
			ID:      "builtin:recon:nikto",
			Pattern: regexp.MustCompile(`\bnikto\b`),
			Action:  core.DecisionApproval,
			Reason:  "Web server vulnerability scanning",
		},
		{
			ID:      "builtin:recon:gobuster",
			Pattern: regexp.MustCompile(`\b(gobuster|dirb|dirbuster|ffuf)\b`),
			Action:  core.DecisionCaution,
			Reason:  "Web directory brute-forcing",
		},
	}...)
}
