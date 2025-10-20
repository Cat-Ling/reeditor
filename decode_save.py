import json
import pickle
import sys
import zipfile
from types import ModuleType

# This script is a standalone version of the decoding logic used in cli.py.
# It serves as a focused tool for debugging or inspecting the contents of a save file.

# Add the project's dummy modules to the Python path
sys.path.insert(0, '.')

# A generic placeholder class to handle unknown Ren'Py objects
class GenericPlaceholder:
    def __init__(self, *args, **kwargs):
        pass
    def __setstate__(self, state):
        if isinstance(state, dict):
            self.__dict__.update(state)
        else:
            self.state = state

# --- Import all dummy modules to ensure their structures are available ---
import renpy.revertable.object
import renpy.ast.ast
import renpy.character.character
import store.store
import store._console.console
import renpy.execution.execution
import renpy.display.layout.layout
import renpy.styledata.styleclass.styleclass
import renpy.rollback.rollback
import renpy.audio.audio.audio
import renpy.display.image.image
# --- End of dummy module imports ---


# A map of all known dummy classes for quick lookup
known_classes = {
    ("renpy.revertable", "RevertableList"): renpy.revertable.object.RevertableList,
    ("renpy.revertable", "RevertableDict"): renpy.revertable.object.RevertableDict,
    ("renpy.revertable", "RevertableSet"): renpy.revertable.object.RevertableSet,
    ("renpy.revertable", "RevertableObject"): renpy.revertable.object.RevertableObject,
    ("renpy.ast", "PyExpr"): renpy.ast.ast.PyExpr,
    ("renpy.character", "HistoryEntry"): renpy.character.character.HistoryEntry,
    ("store", "VoiceInfo"): store.store.VoiceInfo,
    ("store._console", "TracedExpressionsList"): store._console.console.TracedExpressionsList,
    ("renpy.execution", "Delete"): renpy.execution.execution.Delete,
    ("renpy.execution", "Context"): renpy.execution.execution.Context,
    ("renpy.display.layout", "Null"): renpy.display.layout.layout.Null,
    ("renpy.styledata.styleclass", "Style"): renpy.styledata.styleclass.styleclass.Style,
    ("renpy.rollback", "RollbackLog"): renpy.rollback.rollback.RollbackLog,
    ("renpy.rollback", "Rollback"): renpy.rollback.rollback.Rollback,
    ("renpy.audio.audio", "MusicContext"): renpy.audio.audio.audio.MusicContext,
    ("renpy.display.image", "ShownImageInfo"): renpy.display.image.image.ShownImageInfo,
}

_placeholder_cache = {}

class PlaceholderFactory(type):
    """A metaclass to dynamically create and cache placeholder classes."""
    def __new__(cls, module_name, class_name):
        key = (module_name, class_name)
        if key in _placeholder_cache:
            return _placeholder_cache[key]

        # Create the new placeholder class
        new_class = type(class_name, (GenericPlaceholder,), {})
        new_class.__module__ = module_name

        # Create a dynamic module for the class and register it
        if module_name not in sys.modules:
            sys.modules[module_name] = ModuleType(module_name)

        setattr(sys.modules[module_name], class_name, new_class)

        _placeholder_cache[key] = new_class
        return new_class

class CustomUnpickler(pickle.Unpickler):
    """
    An unpickler that uses our custom class lookup to handle Ren'Py objects.
    """
    def find_class(self, module, name):
        if (module, name) in known_classes:
            return known_classes[(module, name)]
        return PlaceholderFactory(module, name)

class CustomJSONEncoder(json.JSONEncoder):
    """
    A JSON encoder that serializes our custom objects into a lossless format,
    preserving their module and class information.
    """
    def default(self, o):
        cls_module = getattr(o.__class__, '__module__', None)
        if cls_module and (cls_module.startswith(('renpy', 'store')) or isinstance(o, GenericPlaceholder)):
            module = o.__class__.__module__
            name = o.__class__.__name__
            state = None
            if isinstance(o, (list, dict)):
                state = type(o)(o)
            elif isinstance(o, set):
                state = {"__set__": list(o)} # Special handling for sets
            elif hasattr(o, '__dict__') and o.__dict__:
                state = o.__dict__
            elif hasattr(o, 'state'):
                state = o.state
            return {"__class__": name, "__module__": module, "__state__": state}
        return super().default(o)

def decode_save_file(save_file_path):
    """
    Decodes a Ren'Py save file and returns the data structure.
    """
    try:
        with zipfile.ZipFile(save_file_path, 'r') as z:
            if 'log' not in z.namelist():
                print(f"Error: 'log' file not found in {save_file_path}", file=sys.stderr)
                return None
            with z.open('log') as f:
                unpickler = CustomUnpickler(f)
                return unpickler.load()
    except FileNotFoundError:
        print(f"Error: File not found at {save_file_path}", file=sys.stderr)
    except zipfile.BadZipFile:
        print(f"Error: Not a valid zip file: {save_file_path}", file=sys.stderr)
    except Exception as e:
        print(f"An unexpected error occurred during decoding: {e}", file=sys.stderr)
    return None

def main():
    """
    Main execution function.
    """
    if len(sys.argv) < 2:
        print(f"Usage: python {sys.argv[0]} <save_file>")
        sys.exit(1)

    save_file_path = sys.argv[1]
    data = decode_save_file(save_file_path)

    if data is not None:
        print(json.dumps(data, indent=2, cls=CustomJSONEncoder))

if __name__ == "__main__":
    main()
