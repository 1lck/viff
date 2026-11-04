"""Regression coverage for draft discovery without any GitHub network calls."""
import hashlib
import json
import os
from pathlib import Path
import runpy
import subprocess
import tempfile
import unittest
from unittest.mock import patch

SCRIPT = Path(__file__).with_name('publish-release.py').resolve()
TAG = 'v0.1.0-alpha.2'


class PublishReleaseTests(unittest.TestCase):
    def exercise(self, existing_draft):
        state = {
            'id': 123, 'tag_name': TAG, 'draft': True, 'assets': [],
            'html_url': f'https://github.com/example/viff/releases/tag/{TAG}',
        }
        created = existing_draft
        calls = []

        def response(args, status=0, value=None):
            return subprocess.CompletedProcess(args, status, json.dumps(value), '')

        def fake_gh(args, **kwargs):
            nonlocal created
            calls.append(args)
            command = args[1:]
            if command[:2] == ['release', 'create']:
                self.assertFalse(created, 'Do not create a duplicate draft')
                self.assertIn('--notes-file', command)
                self.assertNotIn('--generate-notes', command)
                created = True
                # A failed response can still have created the draft remotely.
                return response(args, 1)
            if command[:2] == ['release', 'upload']:
                path = Path(command[3])
                state['assets'].append({
                    'name': path.name, 'size': path.stat().st_size,
                    'digest': 'sha256:' + hashlib.sha256(path.read_bytes()).hexdigest(),
                })
                return response(args)
            if command[:3] == ['api', '-X', 'PATCH']:
                if 'draft=false' in command:
                    state['draft'] = False
                else:
                    state['body'] = next(arg[5:] for arg in command if arg.startswith('body='))
                    state['name'] = next(arg[5:] for arg in command if arg.startswith('name='))
                return response(args, 1)  # Applied despite a server error.
            if command[:3] == ['api', '--paginate', '--slurp']:
                # Put the draft on the second page to exercise pagination.
                return response(args, value=[[], [state] if created else []])
            if command[0] == 'api' and '/tags/' in command[1]:
                return response(args, 1)  # Tag lookup cannot see the draft.
            if command == ['api', 'repos/example/viff/releases/123']:
                return response(args, value=state)
            self.fail(f'Unexpected subprocess: {args}')

        previous = Path.cwd()
        with tempfile.TemporaryDirectory() as directory:
            try:
                os.chdir(directory)
                Path('dist').mkdir()
                for name in [f'viff-{TAG}-{suffix}' for suffix in (
                    'darwin-arm64.dmg', 'windows-amd64-setup.exe',
                    'linux-amd64.deb', 'linux-amd64.tar.gz',
                )] + ['SHA256SUMS']:
                    (Path('dist') / name).write_bytes(b'test asset')
                with patch.dict(os.environ, GH_REPO='example/viff', GITHUB_REF_NAME=TAG), \
                        patch('subprocess.run', side_effect=fake_gh), patch('time.sleep'), \
                        patch('release_notes.load_notes', return_value='Curated bilingual notes'):
                    runpy.run_path(str(SCRIPT))
            finally:
                os.chdir(previous)
        self.assertFalse(state['draft'])
        self.assertEqual(state['body'], 'Curated bilingual notes')
        self.assertEqual(state['name'], f'Viff {TAG}')
        self.assertEqual(len(state['assets']), 5)
        self.assertEqual(sum(c[1:3] == ['release', 'create'] for c in calls), 0 if existing_draft else 1)

    def test_missing_notes_block_all_github_calls(self):
        with patch.dict(os.environ, GH_REPO='example/viff', GITHUB_REF_NAME=TAG), \
                patch('release_notes.load_notes', side_effect=ValueError('Missing notes')), \
                patch('subprocess.run') as remote:
            with self.assertRaisesRegex(ValueError, 'Missing notes'):
                runpy.run_path(str(SCRIPT))
            remote.assert_not_called()

    def test_existing_draft_is_found_without_recreating_it(self):
        self.exercise(existing_draft=True)

    def test_uncertain_create_and_publish_responses_are_reconciled(self):
        self.exercise(existing_draft=False)


if __name__ == '__main__':
    unittest.main()
