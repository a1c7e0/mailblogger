package web

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/image/draw"
)

func (s *Server) detectAssets() {
	s.avatarFile = s.Store.DetectAvatar()
	if s.avatarFile != "" {
		s.Site.Avatar = "/static/" + s.avatarFile
	}
	faviconSVG := filepath.Join(s.Store.ContentDir, "favicon.svg")
	faviconICO := filepath.Join(s.Store.ContentDir, "favicon.ico")
	svgExists := false
	if _, err := os.Stat(faviconSVG); err == nil {
		svgExists = true
	}
	icoExists := false
	if _, err := os.Stat(faviconICO); err == nil {
		icoExists = true
	}
	if svgExists && icoExists {
		return
	}
	if s.avatarFile == "" {
		return
	}
	avatarPath := filepath.Join(s.Store.ContentDir, s.avatarFile)
	data, err := os.ReadFile(avatarPath)
	if err != nil {
		return
	}
	ext := s.avatarFile[strings.LastIndex(s.avatarFile, ".")+1:]
	mime := "image/" + ext
	if ext == "jpg" {
		mime = "image/jpeg"
	}
	if !svgExists {
		s.cachedFaviconSVG = []byte(fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 256 256"><image href="data:%s;base64,%s" width="256" height="256"/></svg>`,
			mime, encodeBase64(data)))
	}
	if !icoExists {
		if img, _, err := image.Decode(bytes.NewReader(data)); err == nil {
			s.cachedFaviconICO = generateICO(img)
		}
	}
}

func generateICO(src image.Image) []byte {
	const size = 32
	resized := image.NewNRGBA(image.Rect(0, 0, size, size))
	draw.NearestNeighbor.Scale(resized, resized.Bounds(), src, src.Bounds(), draw.Over, nil)
	var pngBuf bytes.Buffer
	png.Encode(&pngBuf, resized)
	pngData := pngBuf.Bytes()
	pngSize := uint32(len(pngData))
	headerSize := uint32(6 + 16)
	ico := make([]byte, headerSize+pngSize)
	ico[0] = 0
	ico[1] = 0
	ico[2] = 1
	ico[3] = 0
	ico[4] = 1
	ico[5] = 0
	ico[6] = 0
	ico[7] = 0
	ico[8] = 32
	ico[9] = 0
	ico[10] = 1
	ico[11] = 0
	ico[12] = 32
	ico[13] = 0
	binary.LittleEndian.PutUint32(ico[14:18], pngSize)
	binary.LittleEndian.PutUint32(ico[18:22], headerSize)
	copy(ico[headerSize:], pngData)
	return ico
}

func encodeBase64(data []byte) string {
	const tbl = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/"
	var buf strings.Builder
	for i := 0; i < len(data); i += 3 {
		var b0, b1, b2 byte
		b0 = data[i]
		if i+1 < len(data) {
			b1 = data[i+1]
		}
		if i+2 < len(data) {
			b2 = data[i+2]
		}
		buf.WriteByte(tbl[b0>>2])
		buf.WriteByte(tbl[((b0&3)<<4)|(b1>>4)])
		if i+1 < len(data) {
			buf.WriteByte(tbl[((b1&15)<<2)|(b2>>6)])
		} else {
			buf.WriteByte('=')
		}
		if i+2 < len(data) {
			buf.WriteByte(tbl[b2&63])
		} else {
			buf.WriteByte('=')
		}
	}
	return buf.String()
}
