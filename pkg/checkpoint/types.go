package checkpoint

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// All structs, constants, and interfaces

type Manager struct {
	baseDir     string // Base directory for this session, e.g., /tmp/checkpoint-sessions/a1b2c3d4e5f6g7h8
	overlayDir  string // Directory for overlay layers, e.g., /tmp/checkpoint-sessions/a1b2c3d4e5f6g7h8/overlays
	criuDir     string
	metadataDir string // Directory for metadata files, e.g., /tmp/checkpoint-sessions/a1b2c3d4e5f6g7h8/metadata
	workOverlay string // Current working overlay mount point, e.g., /tmp/checkpoint-sessions/a1b2c3d4e5f6g7h8/work
	originalDir string // Original directory being managed, e.g., /home/user/app-data
	sessionID   string // Unique session identifier, e.g., a1b2c3d4e5f6g7h8
	sandboxMode bool   // Sandbox isolation enabled
}

type Metadata struct {
	ID          string `json:"id"`
	PID         int    `json:"pid"`
	OverlayPath string `json:"overlay_path"`
	CriuPath    string `json:"criu_path"`
	Timestamp   int64  `json:"timestamp"`
	OriginalDir string `json:"original_dir"`
	SessionID   string `json:"session_id"`
}

type SessionInfo struct {
	SessionID   string `json:"session_id"`
	BaseDir     string `json:"base_dir"`
	OriginalDir string `json:"original_dir"`
	WorkOverlay string `json:"work_overlay"`
	SandboxMode bool   `json:"sandbox_mode"`
	CreatedAt   int64  `json:"created_at"`
}

var (
	DefaultSessionsDir = "/tmp/checkpoint-sessions"
	SessionInfoDir     = "/tmp/checkpoint-sessions-info"
)

type dirConfig struct {
	SessionsDir    string `json:"sessions_dir"`
	SessionInfoDir string `json:"session_info_dir"`
}

// init loads configuration: Go automatically calls init functions per package before main.
func init() {
	// Determine config by precedence:
	// 1) Direct environment variable overrides for individual dirs take the highest precedence.
	//    CHECKPOINT_SESSIONS_DIR, CHECKPOINT_SESSION_INFO_DIR
	// 2) Determine config file path by precedence:
	//    a) explicit CHECKPOINT_CONFIG env var
	//    b) binary-side config: ./config.json (same dir as executable)
	//    c) user config: $XDG_CONFIG_HOME/checkpoint-lite/config.json or ~/.checkpoint-lite/config.json
	//    d) system config: /etc/checkpoint-lite/config.json
	// 3) If none found, keep defaults as set above.

	// 1) Direct env var overrides
	if v := os.Getenv("CHECKPOINT_SESSIONS_DIR"); v != "" {
		DefaultSessionsDir = v
	}
	if v := os.Getenv("CHECKPOINT_SESSION_INFO_DIR"); v != "" {
		SessionInfoDir = v
	}

	// 2) Config file path determination

	fileExists := func(p string) bool {
		if p == "" {
			return false
		}
		if _, err := os.Stat(p); err == nil {
			return true
		}
		return false
	}

	// 2.a) explicit env var
	cfgPath := os.Getenv("CHECKPOINT_CONFIG")

	if cfgPath == "" {
		// 2.b) binary-side
		if exe, err := os.Executable(); err == nil {
			exeDir := filepath.Dir(exe)
			p := filepath.Join(exeDir, "config.json")
			if fileExists(p) {
				cfgPath = p
			}
		}
	}

	if cfgPath == "" {
		// 2.c) user config
		if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
			p := filepath.Join(xdg, "checkpoint-lite", "config.json")
			if fileExists(p) {
				cfgPath = p
			}
		} else if home, err := os.UserHomeDir(); err == nil {
			p := filepath.Join(home, ".checkpoint-lite", "config.json")
			if fileExists(p) {
				cfgPath = p
			}
		}
	}

	if cfgPath == "" {
		// 2.d) system config
		p := filepath.Join("/etc", "checkpoint-lite", "config.json")
		if fileExists(p) {
			cfgPath = p
		}
	}

	if cfgPath != "" {
		if data, err := os.ReadFile(cfgPath); err == nil {
			var cfg dirConfig
			if err := json.Unmarshal(data, &cfg); err == nil {
				if cfg.SessionsDir != "" && os.Getenv("CHECKPOINT_SESSIONS_DIR") == "" {
					DefaultSessionsDir = cfg.SessionsDir
				}
				if cfg.SessionInfoDir != "" && os.Getenv("CHECKPOINT_SESSION_INFO_DIR") == "" {
					SessionInfoDir = cfg.SessionInfoDir
				}
			}
		}
	}
}

const SkipMemoryCheckpoint = -1 // User requested to skip memory checkpoint
