package main

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestCorruptionLoad(t *testing.T) {
	appDataDir := getAppDataDir()
	extractRuntimeIfNeeded(appDataDir)

	// We create a corrupted save
	corruptBytes := []byte("PK\x03\x04I'm not a real zip file!")
	os.WriteFile("corrupt.save", corruptBytes, 0644)
	defer os.Remove("corrupt.save")

	body := new(bytes.Buffer)
	writer := multipart.NewWriter(body)
	part, _ := writer.CreateFormFile("savefile", "corrupt.save")
	part.Write(corruptBytes)
	writer.Close()

	req := httptest.NewRequest("POST", "/api/load", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	w := httptest.NewRecorder()
	handleLoad(w, req)

	resp := w.Result()
	if resp.StatusCode != 200 {
		t.Fatalf("Load failed with status %d", resp.StatusCode)
	}

	respBody, _ := io.ReadAll(resp.Body)
	var data JSONMap
	json.Unmarshal(respBody, &data)

	if data["error"] == nil {
		t.Fatalf("Expected error for corrupt zip file but got none")
	}

	errStr := data["error"].(string)
	if !strings.Contains(errStr, "zip") {
		t.Logf("Got expected error: %v", errStr)
	}
}
