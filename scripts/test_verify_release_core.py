import hashlib
import json
from pathlib import Path
import tempfile
import unittest

from verify_release_core import resolve, verify


class ReleaseCoreTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.core = self.root / 'target'
        self.core.mkdir()
        (self.core / 'eos-core').write_bytes(b'signed-current-core')
        self.sha = hashlib.sha256(b'signed-current-core').hexdigest()
        self.pins = {'channel': 'beta', 'version': 'v1.0.0-beta.38',
                     'core_source_commit': 'a' * 40, 'signed_sidecar_run': 123,
                     'core_sha256': {'target': self.sha}}
        self.manifest = {'binary': 'eos-core', 'target': 'target',
                         'source_commit': 'a' * 40, 'sha256': 'sha256:' + self.sha}
        self.file = self.root / 'pins.json'
        self.save()

    def save(self):
        self.file.write_text(json.dumps(self.pins))
        (self.core / 'manifest.json').write_text(json.dumps(self.manifest))

    def test_resolves_frozen_beta_instead_of_main(self):
        self.assertEqual(resolve('v1.0.0-beta.38', self.file), ('a' * 40, '123'))

    def test_rejects_wrong_version_source_and_run(self):
        for version, source, run in [('v1.0.0-beta.39', '', ''),
                                      ('v1.0.0-beta.38', 'main', ''),
                                      ('v1.0.0-beta.38', '', '999')]:
            with self.subTest(version=version, source=source, run=run), self.assertRaises(ValueError):
                resolve(version, self.file, source, run)

    def test_stable_keeps_explicit_core_selection(self):
        self.assertEqual(resolve('v1.0.0', self.root / 'absent', 'stable-source', ''),
                         ('stable-source', ''))

    def test_verifies_actual_frozen_bytes(self):
        self.assertEqual(verify('v1.0.0-beta.38', self.file, self.core),
                         {'cli_version': 'v1.0.0-beta.38', 'core_source_commit': 'a' * 40,
                          'core_target': 'target', 'core_sha256': self.sha})

    def test_rejects_corrupt_binary(self):
        (self.core / 'eos-core').write_bytes(b'old-core')
        with self.assertRaisesRegex(ValueError, 'bytes differ'):
            verify('v1.0.0-beta.38', self.file, self.core)

    def test_rejects_old_core_even_with_matching_manifest(self):
        self.manifest['source_commit'] = 'b' * 40
        self.save()
        with self.assertRaisesRegex(ValueError, 'source differs'):
            verify('v1.0.0-beta.38', self.file, self.core)

    def test_rejects_different_signed_build_of_same_source(self):
        self.pins['core_sha256']['target'] = '0' * 64
        self.save()
        with self.assertRaisesRegex(ValueError, 'digest differs'):
            verify('v1.0.0-beta.38', self.file, self.core)


if __name__ == '__main__':
    unittest.main()
