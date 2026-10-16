"""Prepare an isolated diagnostic texture world and verify BDS pack downloads."""
import argparse
import json
import os
from pathlib import Path
import random
import re
import shutil
import struct
import subprocess
import time
import zlib

PACK_ID = '905d3af4-0b65-43e6-8174-d5a4d8cf4ec7'
WORLD = 'NEMCBridge-ResourcePack-Test'


def png(width, height, pixels):
    def chunk(kind, data):
        return struct.pack('>I', len(data)) + kind + data + struct.pack('>I', zlib.crc32(kind + data))
    rows = b''.join(b'\0' + pixels[y*width*4:(y+1)*width*4] for y in range(height))
    return (b'\x89PNG\r\n\x1a\n' + chunk(b'IHDR', struct.pack('>IIBBBBB', width, height, 8, 6, 0, 0, 0))
            + chunk(b'IDAT', zlib.compress(rows)) + chunk(b'IEND', b''))


def prepare(source, target, pack_count=2):
    executable = 'bedrock_server.exe' if os.name == 'nt' else 'bedrock_server'
    if not (source / executable).is_file():
        raise RuntimeError(f'Missing {executable} in BDS distribution')
    if source == target.resolve():
        raise RuntimeError('BDS source must not be the diagnostic target directory')
    target.mkdir(parents=True, exist_ok=True)
    for name in ('resource_packs', 'behavior_packs', 'definitions', 'config'):
        if not (target / name).exists():
            shutil.copytree(source / name, target / name)
    for path in source.iterdir():
        if path.is_file() and (path.name == executable or path.suffix in ('.dll', '.so')
                              or path.name in ('packetlimitconfig.json', 'profanity_filter.wlist')):
            shutil.copy2(path, target / path.name)
    for name in ('development_behavior_packs', 'development_resource_packs', 'development_skin_packs', 'world_templates'):
        (target / name).mkdir(exist_ok=True)
    for name in ('allowlist.json', 'permissions.json'):
        (target / name).write_text('[]\n', encoding='utf-8')
    properties = (source / 'server.properties').read_text(encoding='utf-8')
    for key, value in {'server-port':'19146', 'server-portv6':'19147', 'enable-lan-visibility':'false',
                       'level-name':WORLD, 'online-mode':'false', 'allow-list':'false',
                       'default-player-permission-level':'member', 'gamemode':'creative',
                       'allow-cheats':'true', 'texturepack-required':'true', 'view-distance':'5',
                       'tick-distance':'4', 'compression-algorithm':'zlib',
                       'block-network-ids-are-hashes':'true'}.items():
        properties, count = re.subn(r'(?m)^' + re.escape(key) + r'=.*$', key + '=' + value, properties)
        if not count:
            properties += '\n' + key + '=' + value + '\n'
    (target / 'server.properties').write_text(properties, encoding='utf-8')
    pack = target / 'resource_packs/NEMCBridge-Diagnostic'
    textures = pack / 'textures/blocks'
    textures.mkdir(parents=True, exist_ok=True)
    manifest = {'format_version':2, 'header':{'name':'NEMCBridge texture probe',
        'description':'Diagnostic: stone becomes cyan/magenta checks', 'uuid':PACK_ID,
        'version':[1,0,0], 'min_engine_version':[1,21,0]}, 'modules':[{'type':'resources',
        'uuid':'2039fc47-2904-4eb1-b60e-a4e2cf4d6749', 'version':[1,0,0]}]}
    (pack / 'manifest.json').write_text(json.dumps(manifest), encoding='utf-8')
    # Clear only the optional legacy manifest from our previous diagnostic pack.
    (pack / 'pack_manifest.json').unlink(missing_ok=True)
    pixels = b''.join(bytes((255,0,255,255) if ((x//4+y//4)%2) else (0,255,255,255))
                      for y in range(16) for x in range(16))
    (textures / 'stone.png').write_bytes(png(16,16,pixels))
    # An unreferenced, valid PNG makes the archive larger than one BDS chunk,
    # so the automated test checks reassembly instead of a one-packet transfer.
    (textures / 'download_probe.png').write_bytes(png(512,512,random.Random(20261016).randbytes(512*512*4)))
    (pack / 'textures/terrain_texture.json').write_text(json.dumps({'resource_pack_name':'NEMCBridge-Diagnostic',
        'texture_name':'atlas.terrain', 'texture_data':{'stone':{'textures':'textures/blocks/stone'}}}), encoding='utf-8')
    world = target / 'worlds' / WORLD
    world.mkdir(parents=True, exist_ok=True)
    packs = [{'pack_id':PACK_ID,'version':[1,0,0]}]
    if pack_count == 2:
        second_id = '34e7fb59-61de-414a-b559-acd0cff7b46d'
        second = target / 'resource_packs/NEMCBridge-Diagnostic-Second'
        (second / 'textures/blocks').mkdir(parents=True, exist_ok=True)
        second_manifest = json.loads(json.dumps(manifest))
        second_manifest['header'].update(uuid=second_id, name='NEMCBridge second texture probe')
        second_manifest['modules'][0]['uuid'] = '91fad1c0-f054-44b2-b9c1-dc33fab4bd7a'
        (second / 'manifest.json').write_text(json.dumps(second_manifest), encoding='utf-8')
        (second / 'textures/blocks/stone.png').write_bytes(png(16,16,pixels))
        packs.append({'pack_id':second_id,'version':[1,0,0]})
    (world / 'world_resource_packs.json').write_text(json.dumps(packs), encoding='utf-8')
    return executable


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--bds-dir', type=Path, required=True)
    parser.add_argument('--prepare-only', action='store_true')
    parser.add_argument('--packs', type=int, choices=(1, 2), default=2, help='include a second independent pack by default')
    parser.add_argument('--native-profile', type=Path, help='optional private imported profile; never copied into source or logs')
    args = parser.parse_args()
    root = Path(__file__).resolve().parents[1]
    target = root / 'runtime/resource-pack-bds-test'
    executable = prepare(args.bds_dir.resolve(), target, args.packs)
    if args.prepare_only:
        print(f'Prepared diagnostic texture server: {target}')
        return
    env = dict(os.environ, LOBBYKEEPER_BDS_RP_TEST='127.0.0.1:19146')
    pattern = '^Test((BDS|WebRTC)(ResourcePack(Skipped|Download)|AutomaticResourcePackDownload)ForNetEase|BuiltinClassicProfileWithBackendDownloads)$'
    if args.native_profile:
        env['NEMCBRIDGE_NATIVE_RP_PROFILE'] = str(args.native_profile.resolve(strict=True))
        pattern = '^Test((BDS|WebRTC)(ResourcePack(Skipped|Download)|AutomaticResourcePackDownload)ForNetEase|BuiltinClassicProfileWithBackendDownloads|NativeResourceProfileWith(BDSAndWebRTC|BackendDownloads))$'
    env['LD_LIBRARY_PATH'] = str(target) + os.pathsep + env.get('LD_LIBRARY_PATH','')
    log = target / 'server-test.log'
    results = target / 'integration-output.log'
    with log.open('w',encoding='utf-8') as output:
        server = subprocess.Popen([str(target/executable)],cwd=target,env=env,stdin=subprocess.PIPE,
            stdout=output,stderr=subprocess.STDOUT,creationflags=getattr(subprocess,'CREATE_NO_WINDOW',0))
        try:
            for _ in range(60):
                if server.poll() is not None:
                    raise RuntimeError(f'Diagnostic BDS exited; see {log}')
                if 'Server started' in log.read_text(encoding='utf-8',errors='replace'):
                    break
                time.sleep(1)
            else:
                raise RuntimeError(f'Diagnostic BDS timed out; see {log}')
            with results.open('w',encoding='utf-8') as test_output:
                result = subprocess.run(['go','test','./internal/integration','-run',
                    pattern,
                    '-count=1','-timeout=120s','-v'],cwd=root,env=env,stdout=test_output,stderr=subprocess.STDOUT)
            if result.returncode:
                raise RuntimeError(f'Resource pack regression failed; see {results}')
            print('PASS: BDS/WebRTC explicit and native automatic downloads, multiple packs/chunks, SHA256, ZIP content, stack and StartGame; default skip retained.')
            if args.native_profile:
                print('PASS: native-only and merged native/backend modes preserve preload metadata and extensions through BDS and WebRTC; StartGame received.')
            print(f'Logs: {target}')
        finally:
            if server.poll() is None:
                server.stdin.write(b'stop\n');server.stdin.flush()
                try:
                    server.wait(timeout=20)
                except subprocess.TimeoutExpired:
                    server.terminate();server.wait(timeout=10)


if __name__ == '__main__':
    main()
