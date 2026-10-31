#!/usr/bin/env python3
"""Run the actual Bash installer against synthetic GitHub responses and archives."""
import hashlib
import io
import json
import os
from pathlib import Path
import subprocess
import tarfile
import tempfile
import unittest

SCRIPT = Path(__file__).with_name('install.sh').resolve()


def release(tag, prerelease=True, draft=False):
    return dict(tag_name=tag, prerelease=prerelease, draft=draft)


class InstallChannels(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory(prefix='eos-install-channel-')
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.bin = self.root / 'tools'
        self.bin.mkdir()
        self.fixtures = self.root / 'fixtures'
        self.fixtures.mkdir()
        curl = self.bin / 'curl'
        curl.write_text('''#!/usr/bin/env python3
import os,sys,pathlib,urllib.parse
args=sys.argv[1:]; url=next(x for x in args if x.startswith('https://')); u=urllib.parse.urlparse(url)
f=pathlib.Path(os.environ['EOS_INSTALL_FIXTURES'])
if '/releases/download/' in u.path: name=u.path.rsplit('/',1)[-1]
elif '/tags/' in u.path: name=u.path.rsplit('/',1)[-1]+'.json'
elif u.path.endswith('/latest'): name='latest.json'
else: name='page'+urllib.parse.parse_qs(u.query).get('page',['1'])[0]+'.json'
p=f/name
if not p.exists(): sys.exit(22)
if '-o' in args: pathlib.Path(args[args.index('-o')+1]).write_bytes(p.read_bytes())
else: sys.stdout.buffer.write(p.read_bytes())
''')
        curl.chmod(0o755)
        self.env = dict(os.environ, PATH=str(self.bin)+os.pathsep+os.environ['PATH'],
                        HOME=str(self.root), EOS_INSTALL_FIXTURES=str(self.fixtures),
                        EOS_INSTALL_DIR=str(self.root/'share/eos'), EOS_INSTALL_BIN=str(self.root/'bin'))
        self.write('latest.json', release('v1.0.0-beta.34', False))
        self.write('page1.json', [release('v1.0.0-beta.9'), release('v1.0.0-beta.100', draft=True),
                   release('v9.0.0', False), release('v1.0.0-beta.34', False),
                   release('v1.0.0-beta.35'), release('v1.0.0-beta.36'), release('v1.0.0-beta.40', False)])
        for tag, pre in [('v1.0.0-beta.34', False), ('v1.0.0-beta.35', True), ('v1.0.0-beta.36', True), ('v1.0.0', False)]:
            self.write(tag+'.json', release(tag, pre))

    def write(self, name, data):
        (self.fixtures/name).write_text(json.dumps(data))

    def run_script(self, *args, success=True):
        result = subprocess.run(['bash',str(SCRIPT),*args],env=self.env,text=True,capture_output=True)
        self.assertEqual(result.returncode == 0, success, result.stdout+result.stderr)
        return result.stdout

    def test_channel_selection(self):
        self.assertEqual(self.run_script('--resolve-only').strip(),'v1.0.0-beta.34 stable')
        self.assertEqual(self.run_script('--channel','beta','--resolve-only').strip(),'v1.0.0-beta.36 beta')
        self.write('latest.json',release('v1.0.0',False))
        self.assertEqual(self.run_script('--resolve-only').strip(),'v1.0.0 stable')
        self.write('latest.json',release('v1.0.0-beta.36'))
        self.run_script('--resolve-only',success=False)

    def test_explicit_version(self):
        self.assertEqual(self.run_script('--version','v1.0.0-beta.34','--resolve-only').strip(),'v1.0.0-beta.34 stable')
        self.assertEqual(self.run_script('--version','1.0.0-beta.36','--resolve-only').strip(),'v1.0.0-beta.36 beta')
        self.run_script('--channel','stable','--version','v1.0.0-beta.36','--resolve-only',success=False)
        self.write('v1.0.0-beta.36.json',release('v1.0.0-beta.36',draft=True))
        self.run_script('--version','v1.0.0-beta.36','--resolve-only',success=False)

    def test_pagination_and_numeric_order(self):
        self.write('page1.json',[release('v1.0.0-beta.9')]*100)
        self.write('page2.json',[release('v1.0.0-beta.10')])
        self.assertEqual(self.run_script('--channel','beta','--resolve-only').strip(),'v1.0.0-beta.10 beta')

    def test_nested_strings_are_not_release_metadata(self):
        item=release('v1.0.0-beta.36');item.update(author={'name':'tag_name','nested':{'draft':True}},body='Quoted "tag_name": "v99.0.0-beta.99" \\ path\n新版本')
        self.write('page1.json',[item])
        self.assertEqual(self.run_script('--channel','beta','--resolve-only').strip(),'v1.0.0-beta.36 beta')
        self.write('v1.0.0-beta.36.json',item)
        self.assertEqual(self.run_script('--version','v1.0.0-beta.36','--resolve-only').strip(),'v1.0.0-beta.36 beta')

    def test_network_malformed_and_empty_fail_without_writes(self):
        for contents in ['[]','{','{"message":"rate limit"}','[{"tag_name":"v1.0.0-beta.36","prerelease":"true","draft":false}]']:
            (self.fixtures/'page1.json').write_text(contents)
            self.run_script('--channel','beta','--resolve-only',success=False)
        (self.fixtures/'page1.json').unlink()
        self.run_script('--channel','beta',success=False)
        self.assertFalse((self.root/'share/eos').exists())

    def make_archive(self, tag, checksum='valid'):
        osname = subprocess.check_output(['uname','-s'],text=True).strip().lower()
        arch = subprocess.check_output(['uname','-m'],text=True).strip()
        arch = 'arm64' if arch in ('arm64','aarch64') else 'amd64'
        name=f'eos-cli_{tag}_{osname}-{arch}.tar.gz'
        archive=self.fixtures/name
        with tarfile.open(archive,'w:gz') as tar:
            content=f'#!/bin/sh\necho "eos {tag} (fixture)"\n'.encode()
            info=tarfile.TarInfo('stage/eos');info.size=len(content);info.mode=0o755
            tar.addfile(info,io.BytesIO(content))
            content=b'fixture-core';info=tarfile.TarInfo('stage/core/fixture/eos-core');info.size=len(content)
            tar.addfile(info,io.BytesIO(content))
        sha=hashlib.sha256(archive.read_bytes()).hexdigest() if checksum=='valid' else '0'*64
        (self.fixtures/'SHA256SUMS.txt').write_text(f'{sha}  {name}\n')

    def test_install_persists_channel_and_rerun_follows_it(self):
        self.make_archive('v1.0.0-beta.36')
        self.run_script('--version','v1.0.0-beta.36')
        marker=self.root/'share/eos/eos-install.json'
        self.assertEqual(json.loads(marker.read_text()),dict(channel='beta',method='script'))
        self.assertTrue((self.root/'bin/eos').is_symlink())
        self.assertEqual(self.run_script('--resolve-only').strip(),'v1.0.0-beta.36 beta')
        self.make_archive('v1.0.0-beta.34')
        self.run_script('--version','v1.0.0-beta.34')
        self.assertEqual(json.loads(marker.read_text())['channel'],'stable')
        self.assertEqual(self.run_script('--resolve-only').strip(),'v1.0.0-beta.34 stable')

    def test_checksum_failure_keeps_existing_install(self):
        self.make_archive('v1.0.0-beta.36')
        self.run_script('--version','v1.0.0-beta.36')
        before=(self.root/'share/eos/eos').read_bytes()
        self.make_archive('v1.0.0-beta.35',checksum='invalid')
        self.run_script('--version','v1.0.0-beta.35',success=False)
        self.assertEqual((self.root/'share/eos/eos').read_bytes(),before)
        self.assertEqual(json.loads((self.root/'share/eos/eos-install.json').read_text())['channel'],'beta')

    def test_managed_path_is_not_modified(self):
        self.run_script('--dir',str(self.root/'Cellar/eos/1.0'),'--channel','beta',success=False)
        self.assertFalse((self.root/'Cellar').exists())
        dest=self.root/'share/eos'; dest.mkdir(parents=True)
        marker=dest/'eos-install.json'
        marker.write_text(json.dumps(dict(channel='beta',method='package-manager')))
        self.run_script('--channel','beta',success=False)
        self.assertEqual(json.loads(marker.read_text())['method'],'package-manager')
        marker.write_text('{}')
        self.run_script('--channel','beta',success=False)



if __name__ == '__main__':
    unittest.main()
