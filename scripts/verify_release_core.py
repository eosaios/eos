#!/usr/bin/env python3
"""Resolve the frozen Beta core and reject stale archive/embedded sidecars."""
import argparse
import hashlib
import json
from pathlib import Path


def beta_pins(version, pins_file):
    if '-beta.' not in version:
        return None
    pins = json.loads(pins_file.read_text())
    if pins['channel'] != 'beta' or pins['version'] != version:
        raise ValueError('Beta version does not match beta-release-pins.json')
    return pins


def resolve(version, pins_file, core_ref='', sidecar_run_id=''):
    pins = beta_pins(version, pins_file)
    if pins:
        if core_ref and core_ref != pins['core_source_commit']:
            raise ValueError('Beta core_ref differs from the frozen source commit')
        if sidecar_run_id and sidecar_run_id != str(pins['signed_sidecar_run']):
            raise ValueError('Beta sidecar run differs from the frozen signed build')
        return pins['core_source_commit'], str(pins['signed_sidecar_run'])
    return core_ref or 'main', sidecar_run_id


def verify(version, pins_file, core_dir):
    pins = beta_pins(version, pins_file)
    manifest = json.loads((core_dir / 'manifest.json').read_text())
    binary = core_dir / manifest['binary']
    if binary.parent != core_dir:
        raise ValueError('Sidecar binary must be inside its target directory')
    actual = hashlib.sha256(binary.read_bytes()).hexdigest()
    if actual != manifest['sha256'].removeprefix('sha256:'):
        raise ValueError('Sidecar bytes differ from the signed manifest')
    if pins:
        if manifest.get('source_commit') != pins['core_source_commit']:
            raise ValueError('Sidecar source differs from the frozen Beta core')
        if actual != pins['core_sha256'][manifest['target']]:
            raise ValueError('Sidecar digest differs from the frozen signed build')
    return {'cli_version': version, 'core_source_commit': manifest.get('source_commit'),
            'core_target': manifest['target'], 'core_sha256': actual}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--version', required=True)
    parser.add_argument('--pins', type=Path, default=Path('beta-release-pins.json'))
    parser.add_argument('--core-ref', default='')
    parser.add_argument('--sidecar-run-id', default='')
    parser.add_argument('--core-dir', type=Path)
    parser.add_argument('--write-pair', type=Path)
    parser.add_argument('--cli-commit')
    args = parser.parse_args()
    if args.core_dir:
        pair = verify(args.version, args.pins, args.core_dir)
        if args.write_pair:
            if not args.cli_commit:
                parser.error('--write-pair requires --cli-commit')
            pair['cli_commit'] = args.cli_commit
            args.write_pair.write_text(json.dumps(pair, indent=2) + '\n')
        print(json.dumps(pair))
    else:
        core_ref, run_id = resolve(args.version, args.pins, args.core_ref, args.sidecar_run_id)
        print('core_ref=' + core_ref)
        print('sidecar_run_id=' + run_id)


if __name__ == '__main__':
    main()
