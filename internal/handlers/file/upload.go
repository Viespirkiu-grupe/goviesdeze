package file

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"goviesdeze/internal/config"
	"goviesdeze/internal/utils"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/gin-gonic/gin"
)

// UploadFile handles file uploads for both local filesystem and S3
func UploadFile(cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		filename := c.Param("filename")
		filePath := utils.ShardPath(filename, cfg.StoragePath)
		var existingSize int64

		if cfg.S3 {
			// S3 upload logic
			key := filePath

			// Check if file exists in S3
			headInput := &s3.HeadObjectInput{
				Bucket: aws.String(cfg.S3Bucket),
				Key:    aws.String(key),
			}
			if headOutput, err := cfg.S3Client.HeadObject(headInput); err == nil {
				existingSize = aws.Int64Value(headOutput.ContentLength)
			}

			// Read request body
			body, err := io.ReadAll(c.Request.Body)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to read request body"})
				return
			}

			// Upload to S3
			putInput := &s3.PutObjectInput{
				Bucket: aws.String(cfg.S3Bucket),
				Key:    aws.String(key),
				Body:   aws.ReadSeekCloser(strings.NewReader(string(body))),
			}
			if _, err := cfg.S3Client.PutObject(putInput); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to upload to S3"})
				return
			}

			byteCount := int64(len(body))
			totalSize := utils.GetUsage() - existingSize + byteCount
			// utils.SetUsage(totalSize)
			utils.AddUsage(-existingSize)
			utils.AddUsage(byteCount)

			c.JSON(http.StatusOK, gin.H{
				"uploaded":  filename,
				"replaced":  existingSize > 0,
				"oldSize":   existingSize,
				"newSize":   byteCount,
				"totalSize": totalSize,
			})
		} else {
			// Local filesystem upload logic
			if err := os.MkdirAll(filepath.Dir(filePath), 0755); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create directory"})
				return
			}

			// Check if file exists
			if stat, err := os.Stat(filePath); err == nil {
				existingSize = stat.Size()
			}

			// Create file
			file, err := os.Create(filePath)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create file"})
				return
			}
			defer file.Close()

			// Copy request body to file
			byteCount, err := io.Copy(file, c.Request.Body)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to write file"})
				return
			}

			totalSize := utils.GetUsage() - existingSize + byteCount
			utils.SetUsage(totalSize)

			c.JSON(http.StatusOK, gin.H{
				"uploaded":  filename,
				"replaced":  existingSize > 0,
				"oldSize":   existingSize,
				"newSize":   byteCount,
				"totalSize": totalSize,
			})
		}
	}
}
