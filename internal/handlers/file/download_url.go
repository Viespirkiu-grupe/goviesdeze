package file

import (
	"crypto/md5"
	"fmt"
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

// DownloadURLRequest represents the request body for download-url endpoint
type DownloadURLRequest struct {
	URL string `json:"url" binding:"required"`
}

// DownloadURL handles downloading files from URLs and storing them
func DownloadURL(cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req DownloadURLRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Missing url field"})
			return
		}

		// Download the file from URL
		resp, err := http.Get(req.URL)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch URL"})
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to fetch %s: %s", req.URL, resp.Status)})
			return
		}

		if cfg.S3 {
			// S3 storage logic
			body, err := io.ReadAll(resp.Body)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to read response body"})
				return
			}

			// Calculate MD5 hash
			hash := md5.Sum(body)
			md5sum := fmt.Sprintf("%x", hash)
			filename := md5sum
			key := utils.ShardPath(filename, cfg.StoragePath)

			// Check if file already exists
			headInput := &s3.HeadObjectInput{
				Bucket: aws.String(cfg.S3Bucket),
				Key:    aws.String(key),
			}

			if headOutput, err := cfg.S3Client.HeadObject(headInput); err == nil {
				// File already exists
				c.JSON(http.StatusOK, gin.H{
					"md5":  md5sum,
					"size": aws.Int64Value(headOutput.ContentLength),
				})
				return
			}

			// Upload to S3
			putInput := &s3.PutObjectInput{
				Bucket:      aws.String(cfg.S3Bucket),
				Key:         aws.String(key),
				Body:        aws.ReadSeekCloser(strings.NewReader(string(body))),
				ContentType: aws.String(resp.Header.Get("Content-Type")),
			}

			if _, err := cfg.S3Client.PutObject(putInput); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to upload to S3"})
				return
			}

			// Update usage
			// utils.SetUsage(utils.GetUsage() + int64(len(body)))
			utils.AddUsage(int64(len(body)))

			c.JSON(http.StatusOK, gin.H{
				"md5":  md5sum,
				"size": int64(len(body)),
			})
		} else {
			// Local filesystem storage logic
			// Create temporary file
			tmpFile, err := os.CreateTemp(cfg.StoragePath, "tmp_*")
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create temporary file"})
				return
			}
			defer os.Remove(tmpFile.Name())

			// Calculate MD5 hash while writing
			hash := md5.New()
			multiWriter := io.MultiWriter(tmpFile, hash)

			if _, err := io.Copy(multiWriter, resp.Body); err != nil {
				tmpFile.Close()
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to write to temporary file"})
				return
			}
			tmpFile.Close()

			md5sum := fmt.Sprintf("%x", hash.Sum(nil))
			filename := md5sum
			finalPath := utils.ShardPath(filename, cfg.StoragePath)

			// Check if file already exists
			if _, err := os.Stat(finalPath); err == nil {
				// File already exists, remove temp file
				os.Remove(tmpFile.Name())
				if stat, err := os.Stat(finalPath); err == nil {
					c.JSON(http.StatusOK, gin.H{
						"md5":  md5sum,
						"size": stat.Size(),
					})
					return
				}
			}

			// Create directory if it doesn't exist
			if err := os.MkdirAll(filepath.Dir(finalPath), 0755); err != nil {
				os.Remove(tmpFile.Name())
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create directory"})
				return
			}

			// Move temp file to final location
			if err := os.Rename(tmpFile.Name(), finalPath); err != nil {
				os.Remove(tmpFile.Name())
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to move file"})
				return
			}

			// Update usage
			if stat, err := os.Stat(finalPath); err == nil {
				// utils.SetUsage(utils.GetUsage() + stat.Size())
				utils.AddUsage(stat.Size())
				c.JSON(http.StatusOK, gin.H{
					"md5":  md5sum,
					"size": stat.Size(),
				})
			} else {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get file stats"})
			}
		}
	}
}
