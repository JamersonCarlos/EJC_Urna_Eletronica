package handlers

import (
	"io"
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
)

const maxPhotoBytes = 5 << 20 // 5 MB

var photoExt = map[string]string{"image/jpeg": ".jpg", "image/png": ".png", "image/webp": ".webp"}

// UploadCandidatePhoto recebe o campo multipart "file" e devolve a URL no Cloudinary.
func (h *Handler) UploadCandidatePhoto(c *gin.Context) {
	if h.Cloud == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "upload não configurado: defina CLOUDINARY_URL no servidor"})
		return
	}
	// folga para os cabeçalhos do multipart
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxPhotoBytes+64<<10)
	file, _, err := c.Request.FormFile("file")
	if err != nil {
		badRequest(c, "envie uma imagem de até 5 MB")
		return
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maxPhotoBytes+1))
	if err != nil || len(data) > maxPhotoBytes {
		badRequest(c, "envie uma imagem de até 5 MB")
		return
	}
	// o tipo é detectado pelo conteúdo, não pela extensão informada
	ext, ok := photoExt[http.DetectContentType(data)]
	if !ok {
		badRequest(c, "formato não suportado: use JPG, PNG ou WEBP")
		return
	}
	url, err := h.Cloud.Upload(c, "candidato"+ext, data)
	if err != nil {
		log.Printf("upload: %v", err)
		c.JSON(http.StatusBadGateway, gin.H{"error": "falha ao enviar a imagem para o Cloudinary"})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"url": url})
}
