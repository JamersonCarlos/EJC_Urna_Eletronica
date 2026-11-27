// Package cloudinary faz upload assinado de imagens pela API REST do Cloudinary.
// A chave secreta fica apenas no backend.
package cloudinary

import (
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

type Client struct {
	cloud, key, secret, folder string
	baseURL                    string
	http                       *http.Client
}

// New interpreta a CLOUDINARY_URL (cloudinary://API_KEY:API_SECRET@CLOUD_NAME).
func New(rawURL, folder string) (*Client, error) {
	u, err := url.Parse(rawURL)
	// o erro não inclui a URL para não vazar o segredo nos logs
	if err != nil || u.Scheme != "cloudinary" || u.Host == "" || u.User == nil {
		return nil, errors.New("CLOUDINARY_URL inválida: use cloudinary://API_KEY:API_SECRET@CLOUD_NAME")
	}
	secret, _ := u.User.Password()
	if u.User.Username() == "" || secret == "" {
		return nil, errors.New("CLOUDINARY_URL sem API key ou API secret")
	}
	return &Client{
		cloud: u.Host, key: u.User.Username(), secret: secret, folder: folder,
		baseURL: "https://api.cloudinary.com",
		http:    &http.Client{Timeout: 30 * time.Second},
	}, nil
}

// sign gera a assinatura: SHA-1 dos parâmetros ordenados seguidos do segredo.
func sign(params map[string]string, secret string) string {
	keys := make([]string, 0, len(params))
	for k := range params {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	pairs := make([]string, len(keys))
	for i, k := range keys {
		pairs[i] = k + "=" + params[k]
	}
	sum := sha1.Sum([]byte(strings.Join(pairs, "&") + secret))
	return hex.EncodeToString(sum[:])
}

// Upload envia a imagem e devolve a URL https pública.
func (c *Client) Upload(ctx context.Context, filename string, data []byte) (string, error) {
	params := map[string]string{"timestamp": strconv.FormatInt(time.Now().Unix(), 10)}
	if c.folder != "" {
		params["folder"] = c.folder
	}
	signature := sign(params, c.secret)

	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	params["api_key"] = c.key
	params["signature"] = signature
	for k, v := range params {
		if err := w.WriteField(k, v); err != nil {
			return "", err
		}
	}
	part, err := w.CreateFormFile("file", filename)
	if err != nil {
		return "", err
	}
	if _, err := part.Write(data); err != nil {
		return "", err
	}
	if err := w.Close(); err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		fmt.Sprintf("%s/v1_1/%s/image/upload", c.baseURL, url.PathEscape(c.cloud)), &body)
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", w.FormDataContentType())
	res, err := c.http.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()

	var out struct {
		SecureURL string `json:"secure_url"`
		Error     struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(&out); err != nil {
		return "", fmt.Errorf("cloudinary: resposta inválida (HTTP %d): %w", res.StatusCode, err)
	}
	if res.StatusCode != http.StatusOK || out.SecureURL == "" {
		return "", fmt.Errorf("cloudinary: HTTP %d: %s", res.StatusCode, out.Error.Message)
	}
	return out.SecureURL, nil
}
