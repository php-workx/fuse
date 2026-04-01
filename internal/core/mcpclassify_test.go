package core

import "testing"

func TestMCPClassify_SafePrefix(t *testing.T) {
	tests := []struct {
		name     string
		toolName string
	}{
		{"read_file", "read_file"},
		{"get_resource", "get_resource"},
		{"list_items", "list_items"},
		{"search_documents", "search_documents"},
		{"describe_instance", "describe_instance"},
		{"show_details", "show_details"},
		{"view_log", "view_log"},
		{"count_records", "count_records"},
		{"check_status", "check_status"},
		{"validate_input", "validate_input"},
		{"verify_signature", "verify_signature"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ClassifyMCPTool(tt.toolName, nil)
			if got != DecisionSafe {
				t.Errorf("ClassifyMCPTool(%q, nil) = %q, want %q", tt.toolName, got, DecisionSafe)
			}
		})
	}
}

func TestMCPClassify_CautionPrefix(t *testing.T) {
	tests := []struct {
		name     string
		toolName string
	}{
		{"create_resource", "create_resource"},
		{"update_record", "update_record"},
		{"modify_settings", "modify_settings"},
		{"set_config", "set_config"},
		{"put_item", "put_item"},
		{"add_user", "add_user"},
		{"enable_feature", "enable_feature"},
		{"configure_service", "configure_service"},
		{"install_package", "install_package"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ClassifyMCPTool(tt.toolName, nil)
			if got != DecisionCaution {
				t.Errorf("ClassifyMCPTool(%q, nil) = %q, want %q", tt.toolName, got, DecisionCaution)
			}
		})
	}
}

func TestMCPClassify_DestructivePrefix(t *testing.T) {
	tests := []struct {
		name     string
		toolName string
	}{
		{"delete_file", "delete_file"},
		{"remove_user", "remove_user"},
		{"destroy_instance", "destroy_instance"},
		{"drop_table", "drop_table"},
		{"purge_queue", "purge_queue"},
		{"revoke_token", "revoke_token"},
		{"disable_service", "disable_service"},
		{"terminate_process", "terminate_process"},
		{"stop_server", "stop_server"},
		{"kill_process", "kill_process"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ClassifyMCPTool(tt.toolName, nil)
			if got != DecisionCaution {
				t.Errorf("ClassifyMCPTool(%q, nil) = %q, want %q", tt.toolName, got, DecisionCaution)
			}
		})
	}
}

func TestMCPClassify_ArgScanning(t *testing.T) {
	tests := []struct {
		name     string
		toolName string
		args     map[string]interface{}
		want     Decision
	}{
		{
			name:     "safe prefix but rm -rf in args",
			toolName: "read_file",
			args:     map[string]interface{}{"command": "rm -rf /tmp/data"},
			want:     DecisionApproval,
		},
		{
			name:     "safe prefix but drop table in args",
			toolName: "get_data",
			args:     map[string]interface{}{"query": "DROP TABLE users"},
			want:     DecisionApproval,
		},
		{
			name:     "safe prefix but drop database in args",
			toolName: "list_items",
			args:     map[string]interface{}{"sql": "drop database production"},
			want:     DecisionApproval,
		},
		{
			name:     "safe prefix but delete from in args",
			toolName: "search_records",
			args:     map[string]interface{}{"query": "DELETE FROM users WHERE 1=1"},
			want:     DecisionApproval,
		},
		{
			name:     "safe prefix but truncate in args",
			toolName: "view_log",
			args:     map[string]interface{}{"command": "TRUNCATE TABLE logs"},
			want:     DecisionApproval,
		},
		{
			name:     "caution prefix with destructive args escalates to approval",
			toolName: "create_resource",
			args:     map[string]interface{}{"init_command": "rm -rf /var/data"},
			want:     DecisionApproval,
		},
		{
			name:     "safe prefix with safe args stays safe",
			toolName: "read_file",
			args:     map[string]interface{}{"path": "/etc/hosts"},
			want:     DecisionSafe,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ClassifyMCPTool(tt.toolName, tt.args)
			if got != tt.want {
				t.Errorf("ClassifyMCPTool(%q, %v) = %q, want %q", tt.toolName, tt.args, got, tt.want)
			}
		})
	}
}

func TestMCPClassify_FallbackCaution(t *testing.T) {
	tests := []struct {
		name     string
		toolName string
	}{
		{"unknown tool", "unknown_tool"},
		{"custom_action", "custom_action"},
		{"do_something", "do_something"},
		{"process_data", "process_data"},
		{"run_task", "run_task"},
		{"no prefix match", "foobar"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ClassifyMCPTool(tt.toolName, nil)
			if got != DecisionCaution {
				t.Errorf("ClassifyMCPTool(%q, nil) = %q, want %q", tt.toolName, got, DecisionCaution)
			}
		})
	}
}

func TestMCPClassify_NoArgs(t *testing.T) {
	// nil args should not panic and should return name-based classification.
	got := ClassifyMCPTool("read_file", nil)
	if got != DecisionSafe {
		t.Errorf("ClassifyMCPTool with nil args = %q, want %q", got, DecisionSafe)
	}

	// Empty args map should also work.
	got = ClassifyMCPTool("delete_resource", map[string]interface{}{})
	if got != DecisionCaution {
		t.Errorf("ClassifyMCPTool with empty args = %q, want %q", got, DecisionCaution)
	}
}

func TestMCPClassify_NestedArgs(t *testing.T) {
	tests := []struct {
		name     string
		toolName string
		args     map[string]interface{}
		want     Decision
	}{
		{
			name:     "destructive pattern in nested map",
			toolName: "read_file",
			args: map[string]interface{}{
				"config": map[string]interface{}{
					"setup": map[string]interface{}{
						"command": "rm -rf /important",
					},
				},
			},
			want: DecisionApproval,
		},
		{
			name:     "destructive pattern in slice",
			toolName: "get_data",
			args: map[string]interface{}{
				"queries": []interface{}{
					"SELECT * FROM users",
					"DROP TABLE sessions",
				},
			},
			want: DecisionApproval,
		},
		{
			name:     "safe nested values",
			toolName: "read_file",
			args: map[string]interface{}{
				"config": map[string]interface{}{
					"path": "/home/user/file.txt",
					"options": []interface{}{
						"verbose",
						"recursive",
					},
				},
			},
			want: DecisionSafe,
		},
		{
			name:     "mixed nested with destructive deep value",
			toolName: "list_items",
			args: map[string]interface{}{
				"filters": []interface{}{
					map[string]interface{}{
						"type":  "cleanup",
						"query": "delete from old_records",
					},
				},
			},
			want: DecisionApproval,
		},
		{
			name:     "numeric and boolean values in nested structure",
			toolName: "read_file",
			args: map[string]interface{}{
				"limit":  42,
				"active": true,
				"nested": map[string]interface{}{
					"count": 100,
				},
			},
			want: DecisionSafe,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ClassifyMCPTool(tt.toolName, tt.args)
			if got != tt.want {
				t.Errorf("ClassifyMCPTool(%q, ...) = %q, want %q", tt.toolName, got, tt.want)
			}
		})
	}
}
