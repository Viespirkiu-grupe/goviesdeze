package file

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"

	"goviesdeze/internal/config"
	"goviesdeze/internal/utils"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/gin-gonic/gin"
	"github.com/h2non/filetype"
)

// GetFile handles file downloads with range request support for both local filesystem and S3
func GetFile(cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		filename := c.Param("filename")
		basePath := utils.ShardPath(filename, cfg.StoragePath)

		if cfg.S3 {
			// S3 download logic
			// Generate candidate paths and test each one
			candidates := utils.GenerateCandidatePaths(basePath)
			var foundKey string
			var headOutput *s3.HeadObjectOutput

			for _, testKey := range candidates {
				headInput := &s3.HeadObjectInput{
					Bucket: aws.String(cfg.S3Bucket),
					Key:    aws.String(testKey),
				}
				if output, err := cfg.S3Client.HeadObject(headInput); err == nil {
					foundKey = testKey
					headOutput = output
					break
				}
			}

			if foundKey == "" {
				c.JSON(http.StatusNotFound, gin.H{"error": "File not found"})
				return
			}

			fileSize := aws.Int64Value(headOutput.ContentLength)
			contentType := aws.StringValue(headOutput.ContentType)
			if contentType == "" {
				contentType = getContentType(foundKey)
			}

			rangeHeader := c.GetHeader("Range")
			if rangeHeader != "" {
				// Handle range request
				start, end, err := parseRange(rangeHeader, fileSize)
				if err != nil {
					c.Header("Content-Range", fmt.Sprintf("bytes */%d", fileSize))
					c.Status(http.StatusRequestedRangeNotSatisfiable)
					return
				}

				getInput := &s3.GetObjectInput{
					Bucket: aws.String(cfg.S3Bucket),
					Key:    aws.String(foundKey),
					Range:  aws.String(fmt.Sprintf("bytes=%d-%d", start, end)),
				}

				output, err := cfg.S3Client.GetObject(getInput)
				if err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get object from S3"})
					return
				}
				defer output.Body.Close()

				c.Header("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, fileSize))
				c.Header("Accept-Ranges", "bytes")
				c.Header("Content-Length", strconv.FormatInt(end-start+1, 10))
				c.Header("Content-Type", contentType)
				c.Status(http.StatusPartialContent)

				io.Copy(c.Writer, output.Body)
			} else {
				// Handle full file request
				getInput := &s3.GetObjectInput{
					Bucket: aws.String(cfg.S3Bucket),
					Key:    aws.String(foundKey),
				}

				output, err := cfg.S3Client.GetObject(getInput)
				if err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get object from S3"})
					return
				}
				defer output.Body.Close()

				c.Header("Content-Length", strconv.FormatInt(fileSize, 10))
				c.Header("Content-Type", contentType)
				c.Header("Accept-Ranges", "bytes")
				c.Status(http.StatusOK)

				io.Copy(c.Writer, output.Body)
			}
		} else {
			// Local filesystem download logic
			candidates := utils.GenerateCandidatePaths(basePath)
			var filePath string
			var fileInfo os.FileInfo

			// Find the file
			for _, candidate := range candidates {
				if info, err := os.Stat(candidate); err == nil {
					filePath = candidate
					fileInfo = info
					break
				}
			}

			if filePath == "" {
				c.JSON(http.StatusNotFound, gin.H{"error": "File not found"})
				return
			}

			fileSize := fileInfo.Size()
			contentType := getContentType(filePath)

			rangeHeader := c.GetHeader("Range")
			if rangeHeader != "" {
				// Handle range request
				start, end, err := parseRange(rangeHeader, fileSize)
				if err != nil {
					c.Header("Content-Range", fmt.Sprintf("bytes */%d", fileSize))
					c.Status(http.StatusRequestedRangeNotSatisfiable)
					return
				}

				file, err := os.Open(filePath)
				if err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to open file"})
					return
				}
				defer file.Close()

				file.Seek(start, 0)
				limitedReader := io.LimitReader(file, end-start+1)

				c.Header("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, fileSize))
				c.Header("Accept-Ranges", "bytes")
				c.Header("Content-Length", strconv.FormatInt(end-start+1, 10))
				c.Header("Content-Type", contentType)
				c.Status(http.StatusPartialContent)

				io.Copy(c.Writer, limitedReader)
			} else {
				// Handle full file request
				file, err := os.Open(filePath)
				if err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to open file"})
					return
				}
				defer file.Close()

				c.Header("Content-Length", strconv.FormatInt(fileSize, 10))
				c.Header("Content-Type", contentType)
				c.Header("Accept-Ranges", "bytes")
				c.Status(http.StatusOK)

				io.Copy(c.Writer, file)
			}
		}
	}
}

// parseRange parses the Range header and returns start and end positions
func parseRange(rangeHeader string, fileSize int64) (int64, int64, error) {
	rangeHeader = strings.TrimPrefix(rangeHeader, "bytes=")
	parts := strings.Split(rangeHeader, "-")

	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("invalid range format")
	}

	start, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return 0, 0, err
	}

	var end int64
	if parts[1] == "" {
		end = fileSize - 1
	} else {
		end, err = strconv.ParseInt(parts[1], 10, 64)
		if err != nil {
			return 0, 0, err
		}
	}

	if start >= fileSize || end >= fileSize || start > end {
		return 0, 0, fmt.Errorf("invalid range")
	}

	return start, end, nil
}

// getContentType determines the content type based on file extension
func getContentType(filePath string) string {
	kind, _ := filetype.MatchFile(filePath)
	if kind != filetype.Unknown {
		return kind.MIME.Value
	}
	return "application/octet-stream"
}
