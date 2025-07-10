package filetransfer

import (
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"

	"github.com/dothash/hemmelig-cli/internal/core"
	"github.com/dothash/hemmelig-cli/internal/network"
	"github.com/dothash/hemmelig-cli/internal/protocol"
)

// RequestSendFile initiates a file transfer by sending a file offer.
func RequestSendFile(conn net.Conn, sharedKey []byte, filePath string, sender core.MessageSender, maxFileSize int64) {
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

	if err := network.SendData(conn, sharedKey, protocol.TypeFileOffer, metaBytes); err != nil {
		sender.SendError(fmt.Errorf("could not send file offer: %w", err))
	}
}

// SendFileChunks sends file content in chunks over the connection.
func SendFileChunks(conn net.Conn, sharedKey []byte, filePath string, sender core.MessageSender) {
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
		if err := network.SendData(conn, sharedKey, protocol.TypeFileChunk, chunk); err != nil {
			sender.SendError(fmt.Errorf("could not send file chunk: %w", err))
			return
		}

		totalBytesSent += int64(bytesRead)
		sender.SendProgress(float64(totalBytesSent) / float64(fileInfo.Size()))
	}

	if err := network.SendData(conn, sharedKey, protocol.TypeFileDone, nil); err != nil {
		sender.SendError(fmt.Errorf("could not send file done message: %w", err))
		return
	}
}
