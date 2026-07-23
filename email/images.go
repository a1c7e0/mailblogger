package email

import (
	"bytes"
	"fmt"
	"image"
	"log"
	"os"
	"path/filepath"
	"sort"

	"mailblogger/blog"

	"github.com/chai2010/webp"
)

type ImageData struct {
	CID         string
	OriginalName string
	Data        []byte
	ContentType string
	PartOrder    int
}

func extByType(ct string) string {
	switch ct {
	case "image/jpeg":
		return ".jpg"
	case "image/png":
		return ".png"
	case "image/webp":
		return ".webp"
	case "image/gif":
		return ".gif"
	}
	return ""
}

func convertToWebp(data []byte, contentType string) ([]byte, string) {
	if contentType == "image/gif" {
		return data, ".gif"
	}
	if contentType == "image/webp" {
		return data, ".webp"
	}
	ext := ""
	switch contentType {
	case "image/jpeg":
		ext = ".jpg"
	case "image/png":
		ext = ".png"
	default:
		return data, ext
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		log.Printf("image decode failed for %s: %v, using original", contentType, err)
		return data, ext
	}
	var buf bytes.Buffer
	if err := webp.Encode(&buf, img, &webp.Options{Quality: 80}); err != nil {
		log.Printf("webp encode failed for %s: %v, using original", contentType, err)
		return data, ext
	}
	return buf.Bytes(), ".webp"
}

func saveArticleImages(store *blog.Store, articleID string, images []ImageData, dir string) (saved []string, cidMap map[string]string) {
	sort.Slice(images, func(i, j int) bool {
		return images[i].PartOrder < images[j].PartOrder
	})
	cidMap = make(map[string]string)
	for i, img := range images {
		data, ext := convertToWebp(img.Data, img.ContentType)
		name := fmt.Sprintf("%d%s", i+1, ext)
		if dir != "" {
			if err := os.WriteFile(filepath.Join(dir, name), data, 0644); err != nil {
				log.Printf("save image %s: %v", name, err)
				continue
			}
		} else {
			if err := store.SaveImage(articleID, name, data); err != nil {
				log.Printf("save image %s: %v", name, err)
				continue
			}
		}
		saved = append(saved, name)
		if img.CID != "" {
			cidMap[img.CID] = fmt.Sprintf("%d", i+1)
		}
	}
	return
}

func saveCommentImages(store *blog.Store, articleID, commentUID string, images []ImageData) ([]string, map[string]string) {
	sort.Slice(images, func(i, j int) bool {
		return images[i].PartOrder < images[j].PartOrder
	})
	var saved []string
	cidMap := make(map[string]string)
	prefix := "c_" + commentUID + "_"
	for i, img := range images {
		data, ext := convertToWebp(img.Data, img.ContentType)
		name := fmt.Sprintf("%s%d%s", prefix, i+1, ext)
		if err := store.SaveImage(articleID, name, data); err != nil {
			log.Printf("save image %s: %v", name, err)
			continue
		}
		saved = append(saved, name)
		if img.CID != "" {
			cidMap[img.CID] = fmt.Sprintf("%d", i+1)
		}
	}
	return saved, cidMap
}
