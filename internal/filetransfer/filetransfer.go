package filetransfer

import (
	"fmt"
	"net"
	"os"
	"path/filepath"

	"github.com/bjarneo/jot/internal/core"
	"io"

	"github.com/bjarneo/jot/internal/crypto"
	"github.com/bjarneo/jot/internal/network"
	"github.com/bjarneo/jot/internal/protocol"
)

// RequestSendFile initiates a file transfer by sending a file offer.
func RequestSendFile(conn net.Conn, sharedKey []byte, filePath string, sender core.MessageSender, maxFileSize int64, recipientID string) {
	file, err := os.Open(filePath)
	if err != nil {
		sender.SendError(fmt.Errorf("could not open file: %w", err))
		return
	}
	defer file.Close()

	fileInfo, err := file.Stat()
	if err != nil {
		sender.SendError(fmt.Errorf("could not get file info: %w", err))
		return
	}

	if fileInfo.Size() > maxFileSize {
		sender.SendFileOfferFailed(fmt.Sprintf("file size (%.2f MB) exceeds the limit (%.2f MB)", float64(fileInfo.Size())/1024/1024, float64(maxFileSize)/1024/1024))
		return
	}

	meta := protocol.FileMetadata{FileName: filepath.Base(filePath), FileSize: fileInfo.Size(), OriginalPath: filePath}
	metaBytes, err := meta.ToJSON()
	if err != nil {
		sender.SendError(fmt.Errorf("could not create metadata: %w", err))
		return
	}

	encryptedMeta, err := crypto.Encrypt(metaBytes, sharedKey)
	if err != nil {
		sender.SendError(fmt.Errorf("could not encrypt metadata: %w", err))
		return
	}

	msg := map[string]interface{}{
		"type":       "file_offer",
		"recipient":  recipientID,
		"metadata":   encryptedMeta,
	}

	if err := network.SendData(conn, msg); err != nil {
		sender.SendError(fmt.Errorf("could not send file offer: %w", err))
	}
}

// SendFileChunks sends file content in chunks over the connection.
func SendFileChunks(conn net.Conn, sharedKey []byte, filePath string, sender core.MessageSender, recipientID string) {
	file, err := os.Open(filePath)
	if err != nil {
		sender.SendError(fmt.Errorf("could not open file for streaming: %w", err))
		return
	}
	defer file.Close()

	fileInfo, _ := file.Stat()
	var totalBytesSent int64
	buffer := make([]byte, 1024*4) // 4KB chunks

	for {
		bytesRead, err := file.Read(buffer)
		if err != nil {
			if err == io.EOF {
				break
			}
			sender.SendError(fmt.Errorf("could not read file chunk: %w", err))
			return
		}

		chunk := buffer[:bytesRead]
		encryptedChunk, err := crypto.Encrypt(chunk, sharedKey)
		if err != nil {
			sender.SendError(fmt.Errorf("could not encrypt chunk: %w", err))
			return
		}

		msg := map[string]interface{}{
			"type":       "file_chunk",
			"recipient":  recipientID,
			"chunk":      encryptedChunk,
		}

		if err := network.SendData(conn, msg); err != nil {
			sender.SendError(fmt.Errorf("could not send file chunk: %w", err))
			return
		}

		totalBytesSent += int64(bytesRead)
		sender.SendProgress(float64(totalBytesSent) / float64(fileInfo.Size()))
	}

	msg := map[string]interface{}{
		"type":      "file_done",
		"recipient": recipientID,
	}
	if err := network.SendData(conn, msg); err != nil {
		sender.SendError(fmt.Errorf("could not send file done message: %w", err))
		return
	}
}
