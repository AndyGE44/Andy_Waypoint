package checkpoint

// All structs, constants, and interfaces

// /tmp/
//  ├── checkpoint-sessions/
//  │   	├── a1b2c3d4e5f6g7h8/      	# App A's session
//  │   	│  	├── overlays/
//  │   	│  	│ 	├── current/
//  │   	│  	│ 	│   ├── upper/			# Overlay upper directory
//  │   	│  	│ 	│   └── work/ 			# Overlay work directory
//  │   	│  	│   └── ckpt-1/        	# Checkpoint ckpt-1
//  │   	│  	│       ├── upper/        	# Filesystem state
//  │   	│  	│       └── work/         	# Work directory
//  │   	│   ├── criu/
//  │   	│   │ 	└── ckpt-1/        	# Checkpoint ckpt-1
//  │   	│  	│       └── *.img        	# CRIU image files
//  │   	│   ├── metadata/			# Checkpoint metadata
//  │   	│   │  └── ckpt-1.json			# "Metadata" for ckpt-1
//  │   	│   └── work/           	# App A works here
//  │   	└── x9y8z7w6v5u4t3s2/         	# App B's session
//  │       	├── overlays/
//  │       	├── criu/
//  │       	├── metadata/
//  │       	└── work/                  	# App B works here
//  └── checkpoint-sessions-info/      	# Global session registry
// 		 ├── a1b2c3d4e5f6g7h8.json			# "SessionInfo" for App A
// 		 └── x9y8z7w6v5u4t3s2.json

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
