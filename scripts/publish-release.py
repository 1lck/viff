"""Publish installable assets, reconciling API errors before retrying writes."""
import hashlib
import json
import os
from pathlib import Path
import subprocess
import time

from release_notes import load_notes

repo = os.environ['GH_REPO']
tag = os.environ['GITHUB_REF_NAME']
endpoint = f'repos/{repo}/releases'
notes_path = Path(__file__).resolve().parent.parent / 'docs/releases' / f'{tag}.md'
notes = load_notes(notes_path, tag, repo)
expected = {
    f'viff-{tag}-darwin-arm64.dmg',
    f'viff-{tag}-darwin-amd64.dmg',
    f'viff-{tag}-windows-amd64-setup.exe',
    f'viff-{tag}-windows-amd64.zip',
    f'viff-{tag}-linux-amd64.deb',
    f'viff-{tag}-linux-amd64.tar.gz',
    'SHA256SUMS',
}
if {p.name for p in Path('dist').iterdir()} != expected:
    raise SystemExit('Release assets do not match the required platform distribution')


def gh(*args):
    return subprocess.run(['gh', *args], text=True, capture_output=True)


release_id = None


def release():
    global release_id
    # The tag endpoint does not reliably expose drafts. Once discovered, address
    # the release by ID; use the authenticated list endpoint to find drafts.
    route = f'{endpoint}/{release_id}' if release_id is not None else f'{endpoint}/tags/{tag}'
    result = gh('api', route)
    if result.returncode == 0:
        state = json.loads(result.stdout)
        release_id = state['id']
        return state
    result = gh('api', '--paginate', '--slurp', f'{endpoint}?per_page=100')
    if result.returncode != 0:
        return None
    for page in json.loads(result.stdout):
        for state in page:
            if state['tag_name'] == tag:
                release_id = state['id']
                return state
    return None


for attempt in range(5):
    state = release()
    if state:
        break
    flags = ['--prerelease'] if '-' in tag else []
    result = gh('release', 'create', tag, '--verify-tag', '--draft',
                '--notes-file', str(notes_path), '--title', f'Viff {tag}', *flags)
    if result.returncode:
        print('Create response uncertain; checking release state before retrying.', flush=True)
    time.sleep(5)
else:
    raise SystemExit('Could not create or retrieve draft release')

# Reconcile existing drafts too: every published description comes from the
# version-controlled notes, even when this is a retry of an earlier attempt.
for attempt in range(5):
    state = release()
    if state and state.get('body') == notes and state.get('name') == f'Viff {tag}':
        break
    if state:
        gh('api', '-X', 'PATCH', f"{endpoint}/{state['id']}",
           '-f', f'body={notes}', '-f', f'name=Viff {tag}')
    time.sleep(5)
else:
    raise SystemExit('Could not verify curated release title and notes')

for path in sorted(Path('dist').iterdir()):
    digest = 'sha256:' + hashlib.sha256(path.read_bytes()).hexdigest()
    for attempt in range(5):
        state = release()
        asset = next((a for a in (state or {}).get('assets', []) if a['name'] == path.name), None)
        if asset and asset.get('digest') == digest and asset['size'] == path.stat().st_size:
            print(f'Verified {path.name}', flush=True)
            break
        result = gh('release', 'upload', tag, str(path), '--clobber')
        if result.returncode:
            print(f'Upload response uncertain for {path.name}; checking stored digest.', flush=True)
        time.sleep(5)
    else:
        raise SystemExit(f'Could not verify uploaded asset: {path.name}')

for attempt in range(5):
    state = release()
    if state and not state['draft']:
        print(state['html_url'])
        break
    gh('api', '-X', 'PATCH', f"{endpoint}/{state['id']}", '-F', 'draft=false') if state else None
    time.sleep(5)
else:
    raise SystemExit('Could not verify release publication')
