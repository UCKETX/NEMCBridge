"""Create a separate Aplok Guns 1.1.8 resource pack for NetEase testing.

The original pack is never edited. This is a pack-specific workaround, not a
general converter for player models or animation controllers.
"""
import argparse
import json
from pathlib import Path
import re
import shutil


PACK_ID = 'd6067ec8-4a70-467b-85c2-a43b8be9cebc'
PLAYER = 'entity/player/ster1ee3ad78.json'


def load_jsonc(path):
    text = path.read_text(encoding='utf-8-sig')
    # Preserve quoted strings, including URLs and escaped quotes.
    text = re.sub(r'"(?:\\.|[^"\\])*"|/\*[\s\S]*?\*/|//[^\r\n]*',
                  lambda match: match[0] if match[0].startswith('"') else '', text)
    return json.loads(text)


def prepare(source, output, version):
    source, output = source.resolve(), output.resolve()
    if source == output or source in output.parents or output in source.parents:
        raise ValueError('Source and output must be separate, non-nested directories')
    if output.exists():
        raise ValueError('Output already exists; choose a new directory')
    manifest = load_jsonc(source / 'manifest.json')
    if (manifest['header']['uuid'].lower() != PACK_ID
            or manifest['header']['version'] != [1, 1, 8]):
        raise ValueError('Only the tested Aplok Guns resource pack 1.1.8 is supported')
    if len(version) != 3 or any(v < 0 for v in version) or version <= [1, 1, 8]:
        raise ValueError('Version must be three non-negative integers greater than 1.1.8')
    if any(path.is_symlink() or (hasattr(path, 'is_junction') and path.is_junction())
           for path in [source, *source.rglob('*')]):
        raise ValueError('Linked pack paths are not supported')
    player = load_jsonc(source / PLAYER)
    if player['minecraft:client_entity']['description']['identifier'] != 'minecraft:player':
        raise ValueError('Expected the tested minecraft:player client entity')
    contents = load_jsonc(source / 'contents.json')
    entries = contents['content']
    if not any(entry.get('path') == PLAYER for entry in entries):
        raise ValueError('Player entity is missing from contents.json')

    shutil.copytree(source, output)
    (output / PLAYER).unlink()
    manifest['header']['version'] = version
    (output / 'manifest.json').write_text(
        json.dumps(manifest, ensure_ascii=False, indent=2) + '\n', encoding='utf-8')
    contents['content'] = [entry for entry in entries if entry.get('path') != PLAYER]
    (output / 'contents.json').write_text(
        json.dumps(contents, ensure_ascii=False, indent=2) + '\n', encoding='utf-8')


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('source', type=Path, help='Original extracted resource pack directory')
    parser.add_argument('output', type=Path, help='New directory for the compatible copy')
    parser.add_argument('--version', default='1.1.12',
                        help='New pack version; use a fresh version after each edit (default: 1.1.12)')
    args = parser.parse_args()
    try:
        version = [int(part) for part in args.version.split('.')]
        prepare(args.source, args.output, version)
    except (OSError, ValueError, KeyError, TypeError) as exc:
        parser.exit(1, f'Cannot prepare pack: {exc}\n')
    print(f'Created: {args.output.resolve()}')
    print(json.dumps({'pack_id': PACK_ID, 'version': version}))
    print('Update world_resource_packs.json and the Guns behavior pack dependency to this version.')
    print('Enable only this copy of the resource pack. Original files have not been edited.')


if __name__ == '__main__':
    main()
