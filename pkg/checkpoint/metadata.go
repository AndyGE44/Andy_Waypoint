package checkpoint

// Metadata serialization/deserialization

import (
	"encoding/json"
	"os"
	"path/filepath"
)

func (m *Manager) saveMetadata(checkpointID string, metadata Metadata) error {
	data, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		return err
	}

	metadataPath := filepath.Join(m.metadataDir, checkpointID+".json")
	return os.WriteFile(metadataPath, data, 0644)
}

func (m *Manager) loadMetadata(checkpointID string) (*Metadata, error) {
	metadataPath := filepath.Join(m.metadataDir, checkpointID+".json")
	data, err := os.ReadFile(metadataPath)
	if err != nil {
		return nil, err
	}

	var metadata Metadata
	err = json.Unmarshal(data, &metadata)
	return &metadata, err
}

func updateSessionEnvironment(sessionID, originalDir, workOverlay string) error {
	sessionInfo, err := loadSessionInfo(sessionID)
	if err != nil {
		return err
	}

	sessionInfo.OriginalDir = originalDir
	sessionInfo.WorkOverlay = workOverlay

	data, err := json.MarshalIndent(sessionInfo, "", "  ")
	if err != nil {
		return err
	}

	sessionFile := filepath.Join(SessionInfoDir, sessionID+".json")
	return os.WriteFile(sessionFile, data, 0644)
}
