import pickletools
import sys
import zipfile

def main():
    """
    Disassembles the pickle stream from the 'log' file inside a Ren'Py save file.
    """
    if len(sys.argv) < 2:
        print(f"Usage: python {sys.argv[0]} <save_file>")
        sys.exit(1)

    save_file_path = sys.argv[1]

    try:
        with zipfile.ZipFile(save_file_path, 'r') as z:
            if 'log' not in z.namelist():
                print(f"Error: 'log' file not found in {save_file_path}", file=sys.stderr)
                sys.exit(1)
            with z.open('log') as f:
                pickletools.dis(f)
    except FileNotFoundError:
        print(f"Error: File not found at {save_file_path}", file=sys.stderr)
        sys.exit(1)
    except zipfile.BadZipFile:
        print(f"Error: Not a valid zip file: {save_file_path}", file=sys.stderr)
        sys.exit(1)
    except Exception as e:
        print(f"An unexpected error occurred: {e}", file=sys.stderr)
        sys.exit(1)


if __name__ == "__main__":
    main()
