package file

import (
	"crypto/md5"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"

	"goviesdeze/internal/config"
	"goviesdeze/internal/utils"

	"github.com/gin-gonic/gin"
)

// DownloadURLRequest represents the request body for download-url endpoint
type DownloadURLRequest struct {
	URL             string `json:"url" binding:"required"`
	FollowRedirects bool   `json:"followRedirects"` // defaults to false
}

// DownloadURL handles downloading files from URLs and storing them
func DownloadURL(cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req DownloadURLRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Missing url field"})
			return
		}

		// Create custom HTTP client
		client := &http.Client{}
		if !req.FollowRedirects {
			client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
				// stop redirects
				return http.ErrUseLastResponse
			}
		}

		// Download the file
		resp, err := client.Get(req.URL)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch URL"})
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			errMsg := fmt.Sprintf("Failed to fetch URL %s: status %d (%s)", req.URL, resp.StatusCode, resp.Status)
			if resp.StatusCode >= 300 && resp.StatusCode < 400 {
				loc := resp.Header.Get("Location")
				if loc != "" {
					errMsg += fmt.Sprintf(", redirect to %s", loc)
				}
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": errMsg})
			return
		}

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
