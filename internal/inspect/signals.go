package inspect

// Signal represents a dangerous pattern detected in a file.
type Signal struct {
	Category string // e.g., "cloud_sdk", "subprocess", "destructive_fs", "http_control_plane", "dynamic_exec", "dynamic_import", "destructive_verb", "cloud_cli"
	Pattern  string // the regex pattern that matched
	Line     int    // line number (1-indexed)
	Match    string // the actual matched text
}
