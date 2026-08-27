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

func TestLoadAndSaveAPI(t *testing.T) {
	appDataDir := getAppDataDir()
	err := extractRuntimeIfNeeded(appDataDir)
	if err != nil {
		t.Fatalf("Failed to extract runtime: %v", err)
	}
	initBridgePool(appDataDir, 1)

	// We need tuqj9q.save
	saveBytes, err := os.ReadFile("tuqj9q.save")
	if err != nil {
		t.Fatalf("Missing test file: %v", err)
	}

	// Test Load
	body := new(bytes.Buffer)
	writer := multipart.NewWriter(body)
	part, _ := writer.CreateFormFile("savefile", "tuqj9q.save")
	part.Write(saveBytes)
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

	if data["error"] != nil {
		t.Fatalf("Load returned error: %v", data["error"])
	}

	roots, ok := data["roots"].(map[string]interface{})
	if !ok {
		t.Fatalf("Roots is not a map")
	}
	rootsDict, _ := roots["__dict__"].([]interface{})
	if len(rootsDict) == 0 {
		t.Fatalf("Roots dict is empty")
	}

	// Modify emma
	for _, entryAny := range rootsDict {
		entry := entryAny.([]interface{})
		if entry[0] == "store.emma" {
			entry[1] = "Edited Emma E2E Test"
			break
		}
	}

	payload := map[string]interface{}{
		"roots": roots,
		"log": data["log"],
		"json_meta": data["json_meta"],
		"extra_info": data["extra_info"],
	}
	payloadBytes, _ := json.Marshal(payload)

	// Test Save
	bodySave := new(bytes.Buffer)
	writerSave := multipart.NewWriter(bodySave)
	partSave, _ := writerSave.CreateFormFile("savefile", "tuqj9q.save")
	partSave.Write(saveBytes)
	writerSave.WriteField("payload", string(payloadBytes))
	writerSave.Close()

	reqSave := httptest.NewRequest("POST", "/api/save", bodySave)
	reqSave.Header.Set("Content-Type", writerSave.FormDataContentType())
	wSave := httptest.NewRecorder()
	handleSave(wSave, reqSave)

	respSave := wSave.Result()
	if respSave.StatusCode != 200 {
		t.Fatalf("Save failed with status %d", respSave.StatusCode)
	}

	if strings.Contains(respSave.Header.Get("Content-Type"), "application/json") {
		respBodySave, _ := io.ReadAll(respSave.Body)
		t.Fatalf("Save returned error: %s", string(respBodySave))
	}

	respBodySave, _ := io.ReadAll(respSave.Body)
	if len(respBodySave) == 0 {
		t.Fatalf("Save returned empty file")
	}
}
