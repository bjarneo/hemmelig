package protocol

import "encoding/json"

// --- Protocol Definition ---

const (
	TypeNickname          byte = 0x00
	TypeText              byte = 0x01
	TypeFileOffer         byte = 0x02
	TypeFileAccept        byte = 0x03
	TypeFileReject        byte = 0x04
	TypeFileChunk         byte = 0x05
	TypeFileDone          byte = 0x06
	TypePublicKeyExchange byte = 0x0A // New type for public key exchange
)

// FileMetadata is sent before the file content itself.
type FileMetadata struct {
	FileName     string `json:"fileName"`
	FileSize     int64  `json:"fileSize"`
	OriginalPath string `json:"originalPath,omitempty"` // Used by the sender to know which file to stream
	SenderID     string `json:"senderID,omitempty"`
}

// ToJSON marshals the FileMetadata to JSON.
func (fm *FileMetadata) ToJSON() ([]byte, error) {
	return json.Marshal(fm)
}

// FromJSON unmarshals JSON into FileMetadata.
func (fm *FileMetadata) FromJSON(data []byte) error {
	return json.Unmarshal(data, fm)
}
