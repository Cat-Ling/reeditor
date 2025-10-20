package main

import (
	"archive/zip"
	"bytes"
	"fmt"
	"html"
	"io"
	"net/http"
	"sort"

	"github.com/nlpodyssey/gopickle/pickle"
)

// renderEditableForm recursively builds an HTML form from the unpickled data.
func renderEditableForm(data interface{}, prefix string) string {
	var formBody string
	switch v := data.(type) {
	case map[interface{}]interface{}:
		formBody += "<ul>"
		for key, value := range v {
			keyStr := fmt.Sprintf("%v", key)
			newPrefix := fmt.Sprintf("%s.%s", prefix, keyStr) // Using dot notation for simplicity
			formBody += fmt.Sprintf("<li><label>%s:</label> %s</li>", html.EscapeString(keyStr), renderEditableForm(value, newPrefix))
		}
		formBody += "</ul>"
	case []interface{}:
		formBody += "<ol>"
		for i, value := range v {
			newPrefix := fmt.Sprintf("%s[%d]", prefix, i)
			formBody += fmt.Sprintf("<li>%s</li>", renderEditableForm(value, newPrefix))
		}
		formBody += "</ol>"
	case string:
		formBody += fmt.Sprintf(`<input type="text" name="%s" value="%s" size="100">`, html.EscapeString(prefix), html.EscapeString(v))
	case int, int64, int32, int16, int8:
		formBody += fmt.Sprintf(`<input type="number" name="%s" value="%v">`, html.EscapeString(prefix), v)
	case float64, float32:
		formBody += fmt.Sprintf(`<input type="number" step="any" name="%s" value="%v">`, html.EscapeString(prefix), v)
	case bool:
		checked := ""
		if v {
			checked = "checked"
		}
		formBody += fmt.Sprintf(`<input type="hidden" name="%s" value="false"><input type="checkbox" name="%s" value="true" %s>`, html.EscapeString(prefix), html.EscapeString(prefix), checked)
	case nil:
		formBody += `<em>nil</em>`
	default:
		formBody += fmt.Sprintf(`<code>%s</code>`, html.EscapeString(fmt.Sprintf("%v", v)))
	}
	return formBody
}

func uploadHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Only POST method is allowed", http.StatusMethodNotAllowed)
		return
	}

	file, _, err := r.FormFile("savefile")
	if err != nil {
		http.Error(w, "Error retrieving the file", http.StatusBadRequest)
		return
	}
	defer file.Close()

	fileBytes, err := io.ReadAll(file)
	if err != nil {
		http.Error(w, "Error reading uploaded file", http.StatusInternalServerError)
		return
	}

	zipReader, err := zip.NewReader(bytes.NewReader(fileBytes), int64(len(fileBytes)))
	if err != nil {
		http.Error(w, "Error reading zip file", http.StatusInternalServerError)
		return
	}

	var logFile *zip.File
	for _, f := range zipReader.File {
		if f.Name == "log" {
			logFile = f
			break
		}
	}

	if logFile == nil {
		http.Error(w, "log file not found in save archive", http.StatusBadRequest)
		return
	}

	logFileReader, err := logFile.Open()
	if err != nil {
		http.Error(w, "Error opening log file", http.StatusInternalServerError)
		return
	}
	defer logFileReader.Close()

	unpickler := pickle.NewUnpickler(logFileReader)
	data, err := unpickler.Load()
	if err != nil {
		http.Error(w, "Error unpickling log file", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprintf(w, `
		<!DOCTYPE html>
		<html>
		<head>
			<title>Ren'Py Save Editor</title>
		</head>
		<body>
			<h2>Edit Save File Data</h2>
			<form action="/save" method="post">
				%s
				<br>
				<input type="submit" value="Save Changes">
			</form>
		</body>
		</html>
	`, renderEditableForm(data, "root"))
}

func saveHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Only POST method is allowed", http.StatusMethodNotAllowed)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "Error parsing form", http.StatusInternalServerError)
		return
	}

	// For demonstration, just print the form data.
	// The actual reconstruction of the object is complex and will be handled next.
	fmt.Println("Received form data:")
	// Sort the keys for consistent output
	var keys []string
	for key := range r.PostForm {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	for _, key := range keys {
		values := r.PostForm[key]
		fmt.Printf("  %s: %v\n", key, values)
	}

	fmt.Fprintf(w, "Changes received. Rebuilding the save file is the next step.")
}

func main() {
	http.HandleFunc("/upload", uploadHandler)
	http.HandleFunc("/save", saveHandler)
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprintf(w, `
			<!DOCTYPE html>
			<html>
			<head>
				<title>Ren'Py Save Editor</title>
			</head>
			<body>
				<h2>Upload Save File</h2>
				<form action="/upload" method="post" enctype="multipart/form-data">
					<input type="file" name="savefile">
					<input type="submit" value="Upload">
				</form>
			</body>
			</html>
		`)
	})

	fmt.Println("Server starting on port 8080...")
	if err := http.ListenAndServe(":8080", nil); err != nil {
		fmt.Printf("Error starting server: %s\n", err)
	}
}
