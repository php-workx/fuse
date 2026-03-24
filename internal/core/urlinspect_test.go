package core

import "testing"

func TestInspectURLs_MetadataEndpoint(t *testing.T) {
	d, _ := InspectCommandURLs("curl http://169.254.169.254/latest/meta-data/")
	if d != DecisionBlocked {
		t.Errorf("got %s, want BLOCKED", d)
	}
}

func TestInspectURLs_Localhost(t *testing.T) {
	d, _ := InspectCommandURLs("curl http://localhost:8080/admin")
	if d != DecisionBlocked {
		t.Errorf("got %s, want BLOCKED", d)
	}
}

func TestInspectURLs_IPv6Loopback(t *testing.T) {
	d, _ := InspectCommandURLs("curl http://[::1]:8080/")
	if d != DecisionBlocked {
		t.Errorf("got %s, want BLOCKED", d)
	}
}

func TestInspectURLs_AzureWireServer(t *testing.T) {
	d, _ := InspectCommandURLs("curl http://168.63.129.16/metadata")
	if d != DecisionBlocked {
		t.Errorf("got %s, want BLOCKED", d)
	}
}

func TestInspectURLs_OracleMetadata(t *testing.T) {
	d, _ := InspectCommandURLs("curl http://192.0.0.192/opc/v2/")
	if d != DecisionBlocked {
		t.Errorf("got %s, want BLOCKED", d)
	}
}

func TestInspectURLs_AWSIPv6Metadata(t *testing.T) {
	d, _ := InspectCommandURLs("curl http://[fd00:ec2::254]/latest/")
	if d != DecisionBlocked {
		t.Errorf("got %s, want BLOCKED", d)
	}
}

func TestInspectURLs_RFC1918(t *testing.T) {
	d, _ := InspectCommandURLs("curl http://10.0.0.1:8500/secrets")
	if d != DecisionCaution {
		t.Errorf("got %s, want CAUTION", d)
	}
}

func TestInspectURLs_CarrierGradeNAT(t *testing.T) {
	d, _ := InspectCommandURLs("curl http://100.64.0.1/")
	if d != DecisionCaution {
		t.Errorf("got %s, want CAUTION", d)
	}
}

func TestInspectURLs_NormalURL(t *testing.T) {
	d, _ := InspectCommandURLs("curl https://api.github.com/repos")
	if d == DecisionBlocked || d == DecisionApproval {
		t.Errorf("got %s, want SAFE or CAUTION for normal URL", d)
	}
}

func TestInspectURLs_InsecureFlag(t *testing.T) {
	d, _ := InspectCommandURLs("curl -k https://example.com")
	if d != DecisionCaution {
		t.Errorf("got %s, want CAUTION for insecure flag", d)
	}
}

func TestInspectURLs_ShellVariable(t *testing.T) {
	d, _ := InspectCommandURLs("curl https://$HOST/api")
	if d != DecisionApproval {
		t.Errorf("got %s, want APPROVAL for shell variable in URL (SEC-001)", d)
	}
}

func TestInspectURLs_RedirectFlag(t *testing.T) {
	d, _ := InspectCommandURLs("curl -L https://untrusted.com/redirect")
	// Should be at least CAUTION due to redirect + non-allowlisted host
	if DecisionSeverity(d) < DecisionSeverity(DecisionCaution) {
		t.Errorf("got %s, want at least CAUTION for redirect flag (SEC-003)", d)
	}
}

func TestInspectURLs_WgetFollowsRedirects(t *testing.T) {
	d, _ := InspectCommandURLs("wget https://untrusted.com/file")
	// wget follows redirects by default
	if DecisionSeverity(d) < DecisionSeverity(DecisionCaution) {
		t.Errorf("got %s, want at least CAUTION for wget (follows redirects by default)", d)
	}
}

func TestInspectURLs_UnknownHostname(t *testing.T) {
	d, _ := InspectCommandURLs("curl https://random-host.tld/")
	if d != DecisionCaution {
		t.Errorf("got %s, want CAUTION for unknown hostname in network command (SEC-004)", d)
	}
}

func TestInspectURLs_BlockedScheme_File(t *testing.T) {
	d, _ := InspectCommandURLs("curl file:///etc/passwd")
	if d != DecisionBlocked {
		t.Errorf("got %s, want BLOCKED for file:// scheme (SEC-011)", d)
	}
}

func TestInspectURLs_BlockedScheme_Gopher(t *testing.T) {
	d, _ := InspectCommandURLs("curl gopher://127.0.0.1:25/")
	if d != DecisionBlocked {
		t.Errorf("got %s, want BLOCKED for gopher:// scheme (SEC-011)", d)
	}
}

func TestInspectURLs_TrailingDotHostname(t *testing.T) {
	d, _ := InspectCommandURLs("curl http://metadata.google.internal./")
	if d != DecisionBlocked {
		t.Errorf("got %s, want BLOCKED for trailing-dot blocked hostname", d)
	}
}

func TestInspectURLs_NonNetworkCommand(t *testing.T) {
	d, _ := InspectCommandURLs("git commit -m 'see http://example.com'")
	// git is not a network command — URL in commit message shouldn't trigger
	if d == DecisionBlocked || d == DecisionApproval {
		t.Errorf("got %s, want SAFE for non-network command with URL in string", d)
	}
}

func TestInspectURLs_MCPArguments(t *testing.T) {
	args := map[string]interface{}{
		"url": "http://169.254.169.254/latest/meta-data/",
	}
	d, _ := InspectURLsInArgs(args)
	if d != DecisionBlocked {
		t.Errorf("got %s, want BLOCKED for metadata URL in MCP args", d)
	}
}

func TestInspectURLs_127Loopback(t *testing.T) {
	d, _ := InspectCommandURLs("curl http://127.0.0.1:9090/")
	if d != DecisionBlocked {
		t.Errorf("got %s, want BLOCKED for 127.0.0.1 (loopback range)", d)
	}
}

func TestInspectURLs_172Private(t *testing.T) {
	d, _ := InspectCommandURLs("curl http://172.16.0.1/api")
	if d != DecisionCaution {
		t.Errorf("got %s, want CAUTION for 172.16.x.x (RFC1918)", d)
	}
}

func TestInspectURLs_192168Private(t *testing.T) {
	d, _ := InspectCommandURLs("curl http://192.168.1.1/admin")
	if d != DecisionCaution {
		t.Errorf("got %s, want CAUTION for 192.168.x.x (RFC1918)", d)
	}
}
