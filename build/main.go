package main

import (
	"archive/zip"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

const SDK_URL = "https://www.renpy.org/dl/8.5.3/renpy-8.5.3-sdk.zip"

func main() {
	fmt.Println("Starting build process...")

	if _, err := os.Stat(filepath.Join(getHome(), "renpy-8.5.3-sdk.zip")); os.IsNotExist(err) {
		fmt.Println("Downloading Ren'Py SDK...")
		err = downloadFile(filepath.Join(getHome(), "renpy-8.5.3-sdk.zip"), SDK_URL)
		if err != nil {
			panic(err)
		}
	}

	fmt.Println("Preparing embedded runtime...")
	err := prepareRuntime(filepath.Join(getHome(), "renpy-8.5.3-sdk.zip"), "embedded_runtime")
	if err != nil {
		panic(err)
	}

	fmt.Println("Packaging runtime for embedding...")
	err = zipDir("embedded_runtime", "runtime.zip")
	if err != nil {
		panic(err)
	}

	fmt.Println("Build completed successfully!")
}

func downloadFile(filepath string, url string) error {

	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	out, err := os.Create(filepath)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, resp.Body)
	return err
}

func prepareRuntime(zipPath string, destDir string) error {
	os.RemoveAll(destDir)
	os.MkdirAll(destDir, 0755)

	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer r.Close()

	targetOS := runtime.GOOS
	targetArch := runtime.GOARCH

	var pyDir string
	var exeName string

	if targetOS == "windows" {
		if targetArch == "amd64" {
			pyDir = "lib/py3-windows-x86_64"
		} else {
			pyDir = "lib/py3-windows-i686"
		}
		exeName = "renpy.exe"
	} else if targetOS == "darwin" {
		if targetArch == "arm64" {
			pyDir = "lib/py3-mac-arm64"
		} else {
			pyDir = "lib/py3-mac-x86_64"
		}
		exeName = "renpy"
	} else {
		if targetArch == "arm64" {
			pyDir = "lib/py3-linux-aarch64"
		} else {
			pyDir = "lib/py3-linux-x86_64"
		}
		exeName = "renpy.sh"
	}

	for _, f := range r.File {
		pathParts := strings.Split(f.Name, "/")
		if len(pathParts) < 2 {
			continue
		}

		relPath := strings.Join(pathParts[1:], "/")

		if strings.HasPrefix(relPath, "renpy/") || strings.HasPrefix(relPath, pyDir+"/") || strings.HasPrefix(relPath, "lib/python") || relPath == exeName || relPath == "renpy.py" {
			destPath := filepath.Join(destDir, relPath)
			if f.FileInfo().IsDir() {
				os.MkdirAll(destPath, 0755)
				continue
			}

			os.MkdirAll(filepath.Dir(destPath), 0755)

			rc, err := f.Open()
			if err != nil {
				return err
			}

			outFile, err := os.OpenFile(destPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
			if err != nil {
				rc.Close()
				return err
			}

			_, err = io.Copy(outFile, rc)
			outFile.Close()
			rc.Close()
			if err != nil {
				return err
			}
		}
	}

	bridgeScript := `
import sys
import json
import traceback
import tempfile
import zipfile
import os

import renpy.compat.pickle as pickle
import renpy.loadsave as loadsave

OBJECT_CACHE = {}
NEXT_OBJECT_ID = 1

def to_json(obj):
    global NEXT_OBJECT_ID
    if isinstance(obj, (int, float, str, bool, type(None))):
        return obj
    elif type(obj) is list:
        return [to_json(i) for i in obj]
    elif type(obj) is tuple:
        return {"__tuple__": [to_json(i) for i in obj]}
    elif type(obj) is set:
        return {"__set__": [to_json(i) for i in obj]}
    elif type(obj) is dict:
        return {"__dict__": [[to_json(k), to_json(v)] for k, v in obj.items()]}
    else:
        obj_id = NEXT_OBJECT_ID
        NEXT_OBJECT_ID += 1
        OBJECT_CACHE[obj_id] = obj

        full_name = f"{obj.__class__.__module__}.{obj.__class__.__name__}"

        if full_name == "builtins.dict":
             return {"__dict__": [[to_json(k), to_json(v)] for k, v in obj.items()]}
        if full_name == "builtins.list":
             return [to_json(i) for i in obj]
        if full_name == "builtins.set":
             return {"__set__": [to_json(i) for i in obj]}

        state = None
        if hasattr(obj, '__getstate__'):
            try: state = obj.__getstate__()
            except: pass
        if state is None and hasattr(obj, '__dict__'):
            state = obj.__dict__

        base_value = None
        if isinstance(obj, list): base_value = list(obj)
        elif isinstance(obj, dict): base_value = dict(obj)
        elif isinstance(obj, set): base_value = set(obj)

        return {
            "__class__": full_name,
            "__id__": obj_id,
            "__state__": to_json(state),
            "__base__": to_json(base_value) if base_value is not None else None
        }

def from_json(data):
    if isinstance(data, (int, float, str, bool, type(None))):
        return data
    elif isinstance(data, list):
        return [from_json(i) for i in data]
    elif isinstance(data, dict):
        if "__tuple__" in data:
            return tuple(from_json(i) for i in data["__tuple__"])
        elif "__set__" in data:
            return set(from_json(i) for i in data["__set__"])
        elif "__dict__" in data:
            return {from_json(k): from_json(v) for k, v in data["__dict__"]}
        elif "__class__" in data and "__id__" in data:
            obj_id = data["__id__"]
            if obj_id in OBJECT_CACHE:
                obj = OBJECT_CACHE[obj_id]
                if "__state__" in data and data["__state__"] is not None:
                    state = from_json(data["__state__"])
                    if hasattr(obj, '__setstate__'):
                        try: obj.__setstate__(state)
                        except Exception:
                            if hasattr(obj, '__dict__') and isinstance(state, dict):
                                obj.__dict__.update(state)
                    elif hasattr(obj, '__dict__') and isinstance(state, dict):
                        obj.__dict__.update(state)
                if "__base__" in data and data["__base__"] is not None:
                    base_val = from_json(data["__base__"])
                    if isinstance(obj, dict) and isinstance(base_val, dict):
                        obj.clear(); obj.update(base_val)
                    elif isinstance(obj, list) and isinstance(base_val, list):
                        obj.clear(); obj.extend(base_val)
                    elif isinstance(obj, set) and isinstance(base_val, set):
                        obj.clear(); obj.update(base_val)
                return obj
            else:
                raise ValueError(f"Object ID {obj_id} not found in cache")
        else:
            return {k: from_json(v) for k, v in data.items()}
    return data

def handle_load(filepath):
    global OBJECT_CACHE, NEXT_OBJECT_ID
    OBJECT_CACHE.clear()
    NEXT_OBJECT_ID = 1

    try:
        if not os.path.exists(filepath):
             return {"error": "File not found"}

        with zipfile.ZipFile(filepath, "r") as zf:
            if "log" not in zf.namelist():
                return {"error": "Invalid Ren'Py save: missing log"}
            log_data = zf.read("log")
            json_meta = {}
            if "json" in zf.namelist():
                try: json_meta = json.loads(zf.read("json").decode("utf-8"))
                except: pass
            extra_info = ""
            if "extra_info" in zf.namelist():
                extra_info = zf.read("extra_info").decode("utf-8")

        roots, log_obj = pickle.loads(log_data)

        return {
            "success": True,
            "json_meta": json_meta,
            "extra_info": extra_info,
            "roots": to_json(roots),
            "log": to_json(log_obj)
        }
    except Exception as e:
        return {"error": str(e), "traceback": traceback.format_exc()}

def handle_save(filepath, out_filepath, payload):
    try:
        # Load the original file first to populate the cache
        load_res = handle_load(filepath)
        if "error" in load_res:
            return load_res

        roots = from_json(payload["roots"])
        log_obj = from_json(payload["log"])

        import io
        logf = io.BytesIO()
        pickle.dump((roots, log_obj), logf)
        new_log_data = logf.getvalue()

        with zipfile.ZipFile(filepath, "r") as z_in:
             with zipfile.ZipFile(out_filepath, "w", zipfile.ZIP_DEFLATED) as z_out:
                  for item in z_in.infolist():
                      if item.filename == "log":
                          z_out.writestr("log", new_log_data)
                      elif item.filename == "json" and "json_meta" in payload:
                          z_out.writestr("json", json.dumps(payload["json_meta"]).encode("utf-8"))
                      elif item.filename == "extra_info" and "extra_info" in payload:
                          z_out.writestr("extra_info", str(payload["extra_info"]).encode("utf-8"))
                      else:
                          z_out.writestr(item, z_in.read(item.filename))
        return {"success": True}
    except Exception as e:
        return {"error": str(e), "traceback": traceback.format_exc()}

print("BRIDGE_READY", file=sys.stderr)
sys.stderr.flush()

for line in sys.stdin:
    line = line.strip()
    if not line:
        continue

    try:
        req = json.loads(line)
        action = req.get("action")

        if action == "load":
            res = handle_load(req.get("filepath"))
        elif action == "save":
            res = handle_save(req.get("filepath"), req.get("out_filepath"), req.get("payload"))
        elif action == "ping":
            res = {"success": True, "pong": True}
        elif action == "quit":
            break
        else:
            res = {"error": "Unknown action"}

        res["req_id"] = req.get("req_id")

        print(json.dumps(res))
        sys.stdout.flush()
    except Exception as e:
        err = {"error": str(e)}
        try: err["req_id"] = json.loads(line).get("req_id")
        except: pass
        print(json.dumps(err))
        sys.stdout.flush()

renpy.quit()
`
	gameDir := filepath.Join(destDir, "bridge_game", "game")
	os.MkdirAll(gameDir, 0755)

	finalScript := "init python:\n"
	for _, line := range strings.Split(bridgeScript, "\n") {
		finalScript += "    " + line + "\n"
	}
	finalScript += "\nlabel start:\n    return\n"

	os.WriteFile(filepath.Join(gameDir, "script.rpy"), []byte(finalScript), 0644)

	return nil
}

func zipDir(src string, dest string) error {
	file, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer file.Close()

	w := zip.NewWriter(file)
	defer w.Close()

	walker := func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}

		f, err := os.Open(path)
		if err != nil {
			return err
		}
		defer f.Close()

		relPath, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		relPath = filepath.ToSlash(relPath)

		fwriter, err := w.Create(relPath)
		if err != nil {
			return err
		}

		_, err = io.Copy(fwriter, f)
		return err
	}
	return filepath.Walk(src, walker)
}

func getHome() string {
	home, _ := os.UserHomeDir()
	return home
}
