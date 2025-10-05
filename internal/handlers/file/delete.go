package file

import (
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

// DeleteFile handles file deletion for both local filesystem and S3
func DeleteFile(cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		filename := c.Param("filename")
		basePath := utils.ShardPath(filename, cfg.StoragePath)

		// Generate candidate paths for the file
		candidates := utils.GenerateCandidatePaths(basePath)

		if cfg.S3 {
			// S3 deletion logic
			key := candidates[0] // Use the first candidate as S3 object key

			// Check if file exists and get its size
			headInput := &s3.HeadObjectInput{
				Bucket: aws.String(cfg.S3Bucket),
				Key:    aws.String(key),
			}

			var size int64
			headOutput, err := cfg.S3Client.HeadObject(headInput)
			if err != nil {
				if strings.Contains(err.Error(), "NotFound") {
					c.JSON(http.StatusNotFound, gin.H{"error": "File not found"})
					return
				}
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to check file existence"})
				return
			}
			size = aws.Int64Value(headOutput.ContentLength)

			// Delete the object
			deleteInput := &s3.DeleteObjectInput{
				Bucket: aws.String(cfg.S3Bucket),
				Key:    aws.String(key),
			}

			if _, err := cfg.S3Client.DeleteObject(deleteInput); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete file from S3"})
				return
			}

			// Update usage
			utils.SetUsage(utils.GetUsage() - size)

			c.JSON(http.StatusOK, gin.H{
				"deleted":    filepath.Base(key),
				"sizeFreed":  size,
			})
		} else {
			// Local filesystem deletion logic
			var filePath string
			var fileInfo os.FileInfo

			// Check each candidate path for existence
			for _, candidate := range candidates {
				if info, err := os.Stat(candidate); err == nil {
					filePath = candidate
					fileInfo = info
					break
				}
			}

			// If no file found, return 404
			if filePath == "" {
				c.JSON(http.StatusNotFound, gin.H{"error": "File not found"})
				return
			}

			// Delete the file
			if err := os.Remove(filePath); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete file"})
				return
			}

			// Update usage
			utils.SetUsage(utils.GetUsage() - fileInfo.Size())

			c.JSON(http.StatusOK, gin.H{
				"deleted":    filepath.Base(filePath),
				"sizeFreed":  fileInfo.Size(),
			})
		}
	}
}
