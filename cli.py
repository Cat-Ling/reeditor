import json
import pickle
import sys
import zipfile
import argparse
import io
from types import ModuleType
import jsonpickle
from jsonpickle.unpickler import Unpickler as JsonPickleUnpickler

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

class CustomJsonUnpickler(JsonPickleUnpickler):
    def find_class(self, module, name):
        try:
            __import__(module)
            return super().find_class(module, name)
        except (ImportError, AttributeError, ModuleNotFoundError):
            return get_placeholder_class(module, name)

def decode(args):
    with zipfile.ZipFile(args.save_file, 'r') as z:
        metadata = {}
        if 'json' in z.namelist():
            with z.open('json') as f:
                metadata = json.load(f)

        log_content = z.read('log')

        pickle_version = None
        if log_content.startswith(b'\x80'):
            pickle_version = log_content[1]

        log_buffer = io.BytesIO(log_content)
        unpickler = CustomUnpickler(log_buffer)
        data = unpickler.load()

        json_friendly_data = json.loads(jsonpickle.encode(data, unpicklable=True))

        output_data = {
            'metadata': metadata,
            'data': json_friendly_data
        }
        if pickle_version is not None:
            output_data['__pickle_version__'] = pickle_version

        print(json.dumps(output_data, indent=2))

def encode(args):
    with open(args.json_file, 'r') as f:
        input_data = json.load(f)

    metadata = input_data.get('metadata', {})
    data_as_dict = input_data.get('data', {})
    data_as_json_str = json.dumps(data_as_dict)

    custom_unpickler = CustomJsonUnpickler()
    data = jsonpickle.decode(data_as_json_str, context=custom_unpickler)

    pickle_version = input_data.get('__pickle_version__', 2)

    pickled_log_buffer = io.BytesIO()
    pickle.dump(data, pickled_log_buffer, protocol=pickle_version)
    pickled_log_buffer.seek(0)

    with zipfile.ZipFile(args.output_file, 'w', zipfile.ZIP_DEFLATED) as new_zip:
        with zipfile.ZipFile(args.save_file, 'r') as original_zip:
            for item in original_zip.infolist():
                if item.filename not in ['log', 'json']:
                    new_zip.writestr(item, original_zip.read(item.filename))

        new_zip.writestr('log', pickled_log_buffer.read())
        new_zip.writestr('json', json.dumps(metadata))

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
