package main

import (
	"archive/zip"
	"bytes"
	"embed"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
)

//go:embed runtime.zip
var runtimeZip embed.FS

//go:embed frontend/*
var frontendFS embed.FS

var (
	bridgePool   chan *BridgeProcess
	reqIDCounter uint64
)

type BridgeProcess struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout io.ReadCloser
	mu     sync.Mutex
}

type JSONMap map[string]interface{}

func main() {
	port := flag.String("port", "8040", "HTTP port to listen on")
	flag.Parse()

	if envPort := os.Getenv("PORT"); envPort != "" {
		port = &envPort
	}

	appDataDir := getAppDataDir()
	err := extractRuntimeIfNeeded(appDataDir)
	if err != nil {
		log.Fatalf("Failed to extract runtime: %v", err)
	}

	initBridgePool(appDataDir, 4)

	http.HandleFunc("/api/load", handleLoad)
	http.HandleFunc("/api/save", handleSave)

	// Serve frontend dir mapped to root
	subFS, _ := fs.Sub(frontendFS, "frontend")
	http.Handle("/", http.FileServer(http.FS(subFS)))

	addr := ":" + *port
	log.Printf("Starting server on http://localhost%s", addr)
	if err := http.ListenAndServe(addr, nil); err != nil {
		log.Fatal(err)
	}
}

func getAppDataDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".renpy_save_editor")
}

func extractRuntimeIfNeeded(dest string) error {
	if _, err := os.Stat(filepath.Join(dest, "bridge_game")); err == nil {
		return nil
	}
	log.Println("Extracting embedded runtime...")
	f, err := runtimeZip.Open("runtime.zip")
	if err != nil { return err }
	defer f.Close()

	stat, _ := f.Stat()
	r, err := zip.NewReader(f.(io.ReaderAt), stat.Size())
	if err != nil { return err }

	for _, zf := range r.File {
		path := filepath.Join(dest, filepath.FromSlash(zf.Name))
		if zf.FileInfo().IsDir() {
			os.MkdirAll(path, 0755)
			continue
		}
		os.MkdirAll(filepath.Dir(path), 0755)
		outFile, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, zf.Mode())
		if err != nil { return err }
		rc, err := zf.Open()
		if err != nil { outFile.Close(); return err }
		io.Copy(outFile, rc)
		outFile.Close(); rc.Close()

		if strings.HasSuffix(zf.Name, ".sh") || strings.HasSuffix(zf.Name, ".exe") || filepath.Base(zf.Name) == "renpy" {
			os.Chmod(path, 0755)
		}
	}
	return nil
}

func initBridgePool(appDataDir string, numWorkers int) {
	bridgePool = make(chan *BridgeProcess, numWorkers)
	for i := 0; i < numWorkers; i++ {
		bp, err := startBridgeProcess(appDataDir)
		if err != nil {
			log.Fatalf("Failed to start bridge worker %d: %v", i, err)
		}
		bridgePool <- bp
	}
}

func startBridgeProcess(appDataDir string) (*BridgeProcess, error) {
	var exeName string
	if runtime.GOOS == "windows" {
		exeName = "renpy.exe"
	} else if runtime.GOOS == "darwin" {
		exeName = "renpy"
	} else {
		exeName = "renpy.sh"
	}

	exePath := filepath.Join(appDataDir, exeName)
	gameDir := filepath.Join(appDataDir, "bridge_game")

	cmd := exec.Command(exePath, gameDir)
	cmd.Env = append(os.Environ(), "SDL_AUDIODRIVER=dummy", "SDL_VIDEODRIVER=dummy")

	stdin, err := cmd.StdinPipe()
	if err != nil { return nil, err }
	stdout, err := cmd.StdoutPipe()
	if err != nil { return nil, err }
	stderr, err := cmd.StderrPipe()
	if err != nil { return nil, err }

	err = cmd.Start()
	if err != nil { return nil, err }

	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := stderr.Read(buf)
			if err != nil { break }
			if bytes.Contains(buf[:n], []byte("BRIDGE_READY")) { break }
		}
		io.Copy(io.Discard, stderr)
	}()

	return &BridgeProcess{
		cmd:    cmd,
		stdin:  stdin,
		stdout: stdout,
	}, nil
}

func (bp *BridgeProcess) ExecAction(req JSONMap) (JSONMap, error) {
	bp.mu.Lock()
	defer bp.mu.Unlock()

	reqID := atomic.AddUint64(&reqIDCounter, 1)
	req["req_id"] = reqID

	reqBytes, _ := json.Marshal(req)
	reqBytes = append(reqBytes, '\n')

	if _, err := bp.stdin.Write(reqBytes); err != nil {
		return nil, err
	}

	dec := json.NewDecoder(bp.stdout)
	var res JSONMap
	if err := dec.Decode(&res); err != nil {
		return nil, err
	}

	return res, nil
}

func handleLoad(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	err := r.ParseMultipartForm(100 << 20)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	file, _, err := r.FormFile("savefile")
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	defer file.Close()

	tmpFile, err := os.CreateTemp("", "renpy_save_*.save")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer os.Remove(tmpFile.Name())

	io.Copy(tmpFile, file)
	tmpFile.Close()

	bp := <-bridgePool
	defer func() { bridgePool <- bp }()

	res, err := bp.ExecAction(JSONMap{
		"action": "load",
		"filepath": tmpFile.Name(),
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(res)
}

func handleSave(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	err := r.ParseMultipartForm(100 << 20)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	file, _, err := r.FormFile("savefile")
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	defer file.Close()

	origFile, err := os.CreateTemp("", "renpy_save_*.save")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer os.Remove(origFile.Name())
	io.Copy(origFile, file)
	origFile.Close()

	payloadStr := r.FormValue("payload")
	var payload JSONMap
	if err := json.Unmarshal([]byte(payloadStr), &payload); err != nil {
		http.Error(w, fmt.Sprintf("Invalid JSON payload: %v", err), http.StatusBadRequest)
		return
	}

	outFile, err := os.CreateTemp("", "renpy_out_*.save")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	outFile.Close()
	defer os.Remove(outFile.Name())

	bp := <-bridgePool
	defer func() { bridgePool <- bp }()

	res, err := bp.ExecAction(JSONMap{
		"action": "save",
		"filepath": origFile.Name(),
		"out_filepath": outFile.Name(),
		"payload": payload,
	})

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if success, _ := res["success"].(bool); !success {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(res)
		return
	}

	w.Header().Set("Content-Disposition", "attachment; filename=modified.save")
	w.Header().Set("Content-Type", "application/octet-stream")
	http.ServeFile(w, r, outFile.Name())
}
