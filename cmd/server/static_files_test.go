package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestSetupStaticFilesServesBrandLogos(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	if err := setupStaticFiles(router); err != nil {
		t.Fatalf("setup static files: %v", err)
	}

	pngSignature := []byte("\x89PNG\r\n\x1a\n")
	for _, path := range []string{"/logo.png", "/logo-dark.png"} {
		t.Run(path, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, path, nil)
			response := httptest.NewRecorder()

			router.ServeHTTP(response, request)

			if response.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
			}
			if contentType := response.Header().Get("Content-Type"); contentType != "image/png" {
				t.Fatalf("Content-Type = %q, want image/png", contentType)
			}
			if !bytes.HasPrefix(response.Body.Bytes(), pngSignature) {
				t.Fatal("response body is not a PNG image")
			}
		})
	}
}
