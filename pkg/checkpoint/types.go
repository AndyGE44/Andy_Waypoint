package checkpoint

// All structs, constants, and interfaces

type Manager struct {
	baseDir     string // Base directory for this session, e.g., /tmp/checkpoint-sessions/a1b2c3d4e5f6g7h8
	overlayDir  string // Directory for overlay layers, e.g., /tmp/checkpoint-sessions/a1b2c3d4e5f6g7h8/overlays
	criuDir     string
	metadataDir string // Directory for metadata files, e.g., /tmp/checkpoint-sessions/a1b2c3d4e5f6g7h8/metadata
	workOverlay string // Current working overlay mount point, e.g., /tmp/checkpoint-sessions/a1b2c3d4e5f6g7h8/work
	originalDir string // Original directory being managed, e.g., /home/user/app-data
	sessionID   string // Unique session identifier, e.g., a1b2c3d4e5f6g7h8
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
	CreatedAt   int64  `json:"created_at"`
}

const (
	DefaultSessionsDir = "/tmp/checkpoint-sessions"
	SessionInfoDir     = "/tmp/checkpoint-sessions-info"
)

const SkipMemoryCheckpoint = -1 // User requested to skip memory checkpoint
