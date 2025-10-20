import json
import pickle
import sys
import zipfile
import argparse
import io
from types import ModuleType

# Add the dummy renpy module to the path
sys.path.insert(0, '.')

# A generic placeholder for any unknown class
class GenericPlaceholder:
    def __init__(self, *args, **kwargs):
        pass
    def __setstate__(self, state):
        if isinstance(state, dict):
            self.__dict__.update(state)
        else:
            self.state = state

# Import all the dummy modules to register the classes
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

# A map of all our known dummy classes
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
    def __new__(cls, module_name, class_name):
        key = (module_name, class_name)
        if key in _placeholder_cache:
            return _placeholder_cache[key]

        # Create the new class
        new_class = type(class_name, (GenericPlaceholder,), {})
        new_class.__module__ = module_name

        # Inject it into a dynamic module
        if module_name not in sys.modules:
            sys.modules[module_name] = ModuleType(module_name)

        setattr(sys.modules[module_name], class_name, new_class)

        _placeholder_cache[key] = new_class
        return new_class

class CustomUnpickler(pickle.Unpickler):
    def find_class(self, module, name):
        if (module, name) in known_classes:
            return known_classes[(module, name)]
        return PlaceholderFactory(module, name)

class CustomJSONEncoder(json.JSONEncoder):
    def default(self, o):
        cls_module = getattr(o.__class__, '__module__', None)
        if cls_module and (cls_module.startswith(('renpy', 'store')) or isinstance(o, GenericPlaceholder)):
            module = o.__class__.__module__
            name = o.__class__.__name__

            state = None
            if isinstance(o, list):
                state = list(o)
            elif isinstance(o, dict):
                state = dict(o)
            elif isinstance(o, set):
                state = {"__set__": list(o)}
            elif hasattr(o, '__dict__') and o.__dict__:
                state = o.__dict__
            elif hasattr(o, 'state'):
                state = o.state

            return {
                "__class__": name,
                "__module__": module,
                "__state__": state
            }
        return super().default(o)

def decode(args):
    save_file_path = args.save_file
    with zipfile.ZipFile(save_file_path, 'r') as z:
        with z.open('log') as f:
            unpickler = CustomUnpickler(f)
            data = unpickler.load()
            print(json.dumps(data, indent=2, cls=CustomJSONEncoder))

def json_object_hook(d):
    if "__class__" in d and "__module__" in d:
        module = d["__module__"]
        name = d["__class__"]
        cls = known_classes.get((module, name)) or PlaceholderFactory(module, name)

        instance = cls.__new__(cls)

        state = d.get("__state__")
        if state is not None:
            if isinstance(instance, list) and isinstance(state, list):
                instance.extend(state)
            elif isinstance(instance, dict) and isinstance(state, dict):
                instance.update(state)
            elif isinstance(instance, set) and isinstance(state, dict) and "__set__" in state:
                instance.update(state["__set__"])
            else:
                instance.__setstate__(state)
        return instance
    return d

def encode(args):
    with open(args.json_file, 'r') as f:
        data = json.load(f, object_hook=json_object_hook)

    pickled_log_buffer = io.BytesIO()
    pickle.dump(data, pickled_log_buffer, protocol=2)
    pickled_log_buffer.seek(0)

    with zipfile.ZipFile(args.output_file, 'w', zipfile.ZIP_DEFLATED) as new_zip:
        with zipfile.ZipFile(args.save_file, 'r') as original_zip:
            for item in original_zip.infolist():
                if item.filename != 'log':
                    new_zip.writestr(item, original_zip.read(item.filename))
        new_zip.writestr('log', pickled_log_buffer.read())
    print(f"Successfully created new save file: {args.output_file}")

def main():
    parser = argparse.ArgumentParser(description="A Ren'Py save editor.")
    subparsers = parser.add_subparsers(dest="command", required=True)

    decode_parser = subparsers.add_parser("decode", help="Decode a Ren'Py save file to lossless JSON.")
    decode_parser.add_argument("save_file", help="The path to the Ren'Py save file.")

    encode_parser = subparsers.add_parser("encode", help="Encode a JSON file back into a Ren'Py save file.")
    encode_parser.add_argument("json_file", help="The path to the input JSON file.")
    encode_parser.add_argument("save_file", help="The path to the original Ren'Py save file (to use as a template).")
    encode_parser.add_argument("output_file", help="The path for the new output save file.")

    args = parser.parse_args()

    if args.command == "decode":
        decode(args)
    elif args.command == "encode":
        encode(args)
    else:
        parser.print_help()

if __name__ == "__main__":
    main()
