# checkpoint-lite

A lightweight checkpoint/restore tool that captures both filesystem and memory state with minimal overhead. 
Built on top of CRIU and OverlayFS for fast, isolated process state management.

## Overview 🌟

`checkpoint-lite` provides a simple interface to checkpoint and restore running processes while capturing both their 
memory state and filesystem changes. Unlike heavyweight container solutions, this tool focuses on minimal overhead 
by directly orchestrating existing kernel features.

### Key Features

- **Hybrid State Capture**: Combines filesystem (OverlayFS) and memory (CRIU) checkpointing
- **Multi-Session Support**: Concurrent usage by multiple applications with isolated sessions
- **Minimal Overhead**: Direct system calls without unnecessary container abstractions
- **Simple CLI**: Straightforward command-line interface for checkpoint operations
- **Session Management**: Automatic cleanup and resource management

## Architecture 🧱

### Design Philosophy

After analysis of existing checkpoint/restore solutions using our analysis tool [StateFork](https://github.com/Alex-XJK/StateFork)
and [StraceTools](https://github.com/Alex-XJK/stracetools), we identified that many traditional solutions often bundle 
unnecessary features like network isolation, security policies, and registry operations. 
`checkpoint-lite` takes a minimalist approach:

1. **Filesystem State**: Uses OverlayFS to capture directory changes without copying entire filesystems
2. **Memory State**: Leverages CRIU for process memory and execution state
3. **Isolation**: Session-based isolation instead of full containerization
4. **Performance**: Direct tool orchestration minimizes call overhead

### Core Components

```
┌─────────────────┐    ┌─────────────────┐
│   Filesystem    │    │     Memory      │
│   (OverlayFS)   │    │     (CRIU)      │
└─────────────────┘    └─────────────────┘
         │                       │
         └───────────┬───────────┘
                     │
            ┌─────────────────┐
            │ checkpoint-lite │
            │   Session Mgr   │
            └─────────────────┘
```

- **OverlayFS Integration**: Creates layered filesystem views with minimal storage overhead
- **CRIU Orchestration**: Manages process memory dumping and restoration
- **Session Manager**: Handles concurrent usage and resource isolation

### Go Language Technology Decision
The tool is implemented in Go for its simplicity, performance, and strong concurrency support.
See [our architecture decision record](./tech_selection_note.md) for more details on why Go was chosen.

## Installation 🔧

### Prerequisites

- Linux system with root privileges
- CRIU installed and configured
- OverlayFS support (most modern Linux distributions)
- Go 1.23 (for building from source)

### Install CRIU

```bash
# Ubuntu/Debian
sudo apt-get install criu
# or go to https://launchpad.net/~criu/+archive/ubuntu/ppa

# Verify installation
sudo criu check
```

### Build from Source

```bash
git clone https://github.com/Alex-XJK/checkpoint-lite.git
cd checkpoint-lite
go build -o checkpoint-lite
```

## Usage 🗂

### 1. Initialize Environment

Create a managed environment for your application:

```bash
sudo ./checkpoint-lite init /path/to/your/workspace
```

Output:
```
Environment initialized!
Session ID: a1b2c3d4e5f6g7h8
Work in this directory: /tmp/checkpoint-sessions/a1b2c3d4e5f6g7h8/work

Save the session ID for future operations!
```

**Important**: Save the session ID and work in the provided directory.

### 2. Run Your Application

```bash
cd /tmp/checkpoint-sessions/a1b2c3d4e5f6g7h8/work
./your-application &
# Note the PID, e.g., 1234
```

### 3. Create Checkpoints

```bash
sudo ./checkpoint-lite create a1b2c3d4e5f6g7h8 1234 checkpoint-name
```
With the help of goroutines, this command runs the CRIU dump and OverlayFS snapshot in parallel,
reducing 40% of the time compared to sequential execution in our tests.

### 4. Restore from Checkpoint

```bash
sudo ./checkpoint-lite restore a1b2c3d4e5f6g7h8 checkpoint-name
```

### 5. List Available Checkpoints

```bash
sudo ./checkpoint-lite list a1b2c3d4e5f6g7h8
```

### 6. Clean Up Session

```bash
sudo ./checkpoint-lite cleanup a1b2c3d4e5f6g7h8
```
If this basic version of the cleanup command fails, our **checkpoint-lite** will automatically instruct you on 
further actions. Namely, you can use:
- `--interactive` to get more information about the failure, or 
- `--force` to forcefully remove and umount all the related resources.

## Example Workflow 🧩

```bash
# Initialize environment
sudo ./checkpoint-lite init /home/user/myproject
## Environment initialized!
## Session ID: abc123def456
## Work in this directory: /tmp/checkpoint-sessions/abc123def456/work
##
## Save the session ID for future operations!

# Run application in managed directory
cd /tmp/checkpoint-sessions/abc123def456/work
./my-simulator --config config.json &
## [1] 5678

# Create checkpoints after some computation
sudo ./checkpoint-lite create abc123def456 5678 simulation-step-100
## Checkpoint 'simulation-step-100' created successfully

# Continue running, create another checkpoint
sudo ./checkpoint-lite create abc123def456 5678 simulation-step-200
## Checkpoint 'simulation-step-200' created successfully

# List available checkpoints
sudo ./checkpoint-lite list abc123def456
## Available checkpoints:
##   simulation-step-100
##   simulation-step-200

# Restore to earlier state
sudo ./checkpoint-lite restore abc123def456 simulation-step-100
## Checkpoint 'simulation-step-100' restored, new PID: 5678

# Clean up when done
sudo ./checkpoint-lite cleanup abc123def456
## Session 'abc123def456' cleaned up successfully
```

## Directory Structure 🗃

```
/tmp/
 ├── checkpoint-sessions/
 │   	├── a1b2c3d4e5f6g7h8/       # App A's session
 │   	│  	├── overlays/
 │   	│  	│ 	├── current/
 │   	│  	│ 	│   ├── upper/	    # Overlay upper directory
 │   	│  	│ 	│   └── work/ 	    # Overlay work directory
 │   	│  	│   └── ckpt-1/         # Checkpoint ckpt-1
 │   	│  	│       ├── upper/          # Filesystem state
 │   	│  	│       └── work/           # Work directory
 │   	│   ├── criu/
 │   	│   │ 	└── ckpt-1/             # Checkpoint ckpt-1
 │   	│  	│       └── *.img           # CRIU image files
 │   	│   ├── metadata/               # Checkpoint metadata
 │   	│   │  └── ckpt-1.json              # "Metadata" for ckpt-1
 │   	│   └── work/           	# App A works here
 │   	└── x9y8z7w6v5u4t3s2/       # App B's session
 │       	├── overlays/
 │       	├── criu/
 │       	├── metadata/
 │       	└── work/
 └── checkpoint-sessions-info/      # Global session registry
		 ├── a1b2c3d4e5f6g7h8.json  # "SessionInfo" for App A
		 └── x9y8z7w6v5u4t3s2.json
```

## Technical Details ⌨️

### Filesystem Checkpointing

- **Lower Layer**: Original workspace (read-only)
- **Upper Layer**: Application changes (copy-on-write)
- **Checkpoint**: Snapshot of upper layer at checkpoint time
- **Restore**: Replace current upper layer with checkpoint snapshot

### Memory Checkpointing

- **CRIU Dump**: Captures process memory, file descriptors, and execution state
- **Process Management**: Handles PID conflicts during restore
- **State Consistency**: Coordinates filesystem and memory restoration

### Session Isolation

Each session gets:
- Unique randomly generated session ID
- Isolated directory structure
- Independent OverlayFS mounts
- Separate checkpoint namespaces

## Limitations

- Requires root privileges (CRIU and OverlayFS requirement)
- Linux-specific (depends on CRIU and OverlayFS)
- Single-process focus (no multi-process trees yet)
- Network connections may not survive checkpoint/restore
