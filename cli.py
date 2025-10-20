import json
import pickle
import sys
import zipfile
import argparse
import io
from types import ModuleType

class RevertableList(list):
    def __setstate__(self, state):
        if isinstance(state, list):
            self.extend(state)
        elif isinstance(state, dict):
            self.__dict__.update(state)
        else:
            self.state = state

class RevertableDict(dict):
    def __setstate__(self, state):
        if isinstance(state, dict):
            self.update(state)
        else:
            self.state = state

class RevertableSet(set):
    def __setstate__(self, state):
        if isinstance(state, set):
            self.update(state)
        elif isinstance(state, dict):
            self.__dict__.update(state)
        else:
            self.state = state

class GenericPlaceholder:
    def __init__(self, *args, **kwargs):
        pass
    def __setstate__(self, state):
        if isinstance(state, dict):
            self.__dict__.update(state)
        else:
            self.state = state

_placeholder_cache = {}

def get_placeholder_class(module_name, class_name):
    key = (module_name, class_name)
    if key in _placeholder_cache:
        return _placeholder_cache[key]

    base_classes = {
        "RevertableList": RevertableList,
        "RevertableDict": RevertableDict,
        "RevertableSet": RevertableSet,
    }
    base_class = base_classes.get(class_name, GenericPlaceholder)
    new_class = type(class_name, (base_class,), {})
    new_class.__module__ = module_name

    # Create nested modules if they don't exist
    parts = module_name.split('.')
    for i in range(1, len(parts) + 1):
        sub_module_name = '.'.join(parts[:i])
        if sub_module_name not in sys.modules:
            new_mod = ModuleType(sub_module_name)
            sys.modules[sub_module_name] = new_mod
            if i > 1:
                parent_module_name = '.'.join(parts[:i-1])
                parent_module = sys.modules[parent_module_name]
                setattr(parent_module, parts[i-1], new_mod)

    final_module = sys.modules[module_name]
    setattr(final_module, class_name, new_class)

    _placeholder_cache[key] = new_class
    return new_class

class CustomUnpickler(pickle.Unpickler):
    def find_class(self, module, name):
        return get_placeholder_class(module, name)

def to_json_friendly(data):
    cls_module = getattr(data.__class__, '__module__', None)
    if isinstance(data, GenericPlaceholder) or (cls_module and (cls_module.startswith('renpy') or cls_module.startswith('store'))):
        state = None
        if isinstance(data, list):
            state = [to_json_friendly(x) for x in data]
        elif isinstance(data, set):
            state = {'__set__': [to_json_friendly(x) for x in data]}
        elif isinstance(data, dict):
            if any(not isinstance(k, (str, int, float, bool, type(None))) for k in data.keys()):
                state = {'__dict_pairs__': [[to_json_friendly(k), to_json_friendly(v)] for k, v in data.items()]}
            else:
                state = {k: to_json_friendly(v) for k, v in data.items()}
        elif hasattr(data, '__dict__') and data.__dict__:
            state = to_json_friendly(data.__dict__)
        elif hasattr(data, 'state'):
            state = to_json_friendly(data.state)

        return {
            "__class__": data.__class__.__name__,
            "__module__": cls_module,
            "__state__": state
        }

    if isinstance(data, (str, int, float, bool, type(None))):
        return data
    if isinstance(data, tuple):
        return {'__tuple__': [to_json_friendly(x) for x in data]}
    if isinstance(data, list):
        return [to_json_friendly(x) for x in data]
    if isinstance(data, set):
        return {'__set__': [to_json_friendly(x) for x in data]}
    if isinstance(data, dict):
        if any(not isinstance(k, (str, int, float, bool, type(None))) for k in data.keys()):
            return {'__dict_pairs__': [[to_json_friendly(k), to_json_friendly(v)] for k, v in data.items()]}
        return {k: to_json_friendly(v) for k, v in data.items()}

    return str(data)

def decode(args):
    with zipfile.ZipFile(args.save_file, 'r') as z:
        with z.open('log') as f:
            unpickler = CustomUnpickler(f)
            data = unpickler.load()
            json_friendly_data = to_json_friendly(data)
            print(json.dumps(json_friendly_data, indent=2))

def json_object_hook(d):
    if '__tuple__' in d:
        return tuple(d['__tuple__'])
    if '__set__' in d:
        return set(d['__set__'])
    if '__dict_pairs__' in d:
        return dict(d['__dict_pairs__'])

    if "__class__" in d and "__module__" in d:
        cls = get_placeholder_class(d["__module__"], d["__class__"])
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
