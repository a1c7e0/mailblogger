package email

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	"io"
	"log"
	"mime"
	"mime/multipart"
	"net/mail"
	"os"
	"path/filepath"
	"sort"
	"strings"

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

func extractImagesFromParsed(msg *mail.Message) []ImageData {
	_, params, err := getContentType(msg)
	if err != nil {
		return nil
	}
	boundary := params["boundary"]
	if boundary == "" {
		return nil
	}
	return extractImagesFromMultipart(msg.Body, boundary, 0)
}

func extractImagesFromMultipart(body io.Reader, boundary string, depth int) []ImageData {
	if boundary == "" || depth >= maxMultipartDepth {
		return nil
	}
	mr := multipart.NewReader(body, boundary)
	var images []ImageData
	order := len(images)
	for {
		part, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			break
		}
		ct, params, _ := mime.ParseMediaType(part.Header.Get("Content-Type"))
		if ct == "" {
			continue
		}
		if strings.HasPrefix(ct, "multipart/") {
			if subBoundary := params["boundary"]; subBoundary != "" {
				sub := extractImagesFromMultipart(part, subBoundary, depth+1)
				images = append(images, sub...)
			}
			continue
		}
		if !strings.HasPrefix(ct, "image/") {
			continue
		}
		order++
		data, _ := io.ReadAll(part)
		if len(data) == 0 {
			continue
		}
		enc := strings.ToLower(part.Header.Get("Content-Transfer-Encoding"))
		var decoded []byte
		switch enc {
		case "base64":
			raw := strings.ReplaceAll(string(data), "\r\n", "")
			raw = strings.ReplaceAll(raw, "\n", "")
			if d, err := base64.StdEncoding.DecodeString(raw); err == nil {
				decoded = d
			} else {
				decoded = data
			}
		default:
			decoded = data
		}
		if len(decoded) == 0 {
			continue
		}
		cid := part.Header.Get("Content-ID")
		cid = strings.Trim(cid, "<>")
		filename := part.Header.Get("Content-Disposition")
		if idx := strings.Index(filename, "filename=\""); idx >= 0 {
			filename = filename[idx+9:]
			filename = strings.Split(filename, "\"")[0]
		} else {
			filename = fmt.Sprintf("img%d", order)
			if ext := extByType(ct); ext != "" {
				filename += ext
			}
		}
		images = append(images, ImageData{
			CID:          cid,
			OriginalName: filename,
			Data:         decoded,
			ContentType:  ct,
			PartOrder:    order,
		})
		log.Printf("extractImagesFromMultipart: extracted image %d at depth=%d", order, depth)
	}
	return images
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

func saveCommentImages(store *blog.Store, articleID, commentUID string, images []ImageData) []string {
	sort.Slice(images, func(i, j int) bool {
		return images[i].PartOrder < images[j].PartOrder
	})
	var saved []string
	prefix := "c_" + commentUID + "_"
	for i, img := range images {
		data, ext := convertToWebp(img.Data, img.ContentType)
		name := fmt.Sprintf("%s%d%s", prefix, i+1, ext)
		if err := store.SaveImage(articleID, name, data); err != nil {
			log.Printf("save image %s: %v", name, err)
			continue
		}
		saved = append(saved, name)
	}
	return saved
}
