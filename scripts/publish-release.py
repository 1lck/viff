"""Publish installable assets, reconciling API errors before retrying writes."""
import hashlib
import json
import os
from pathlib import Path
import subprocess
import time

repo = os.environ['GH_REPO']
tag = os.environ['GITHUB_REF_NAME']
endpoint = f'repos/{repo}/releases'
expected = {
    f'viff-{tag}-darwin-arm64.dmg',
    f'viff-{tag}-windows-amd64-setup.exe',
    f'viff-{tag}-linux-amd64.deb',
    f'viff-{tag}-linux-amd64.tar.gz',
    'SHA256SUMS',
}
if {p.name for p in Path('dist').iterdir()} != expected:
    raise SystemExit('Release assets do not match the required three-platform distribution')


def gh(*args):
    return subprocess.run(['gh', *args], text=True, capture_output=True)


def release():
    result = gh('api', f'{endpoint}/tags/{tag}')
    return json.loads(result.stdout) if result.returncode == 0 else None


for attempt in range(5):
    state = release()
    if state:
        break
    flags = ['--prerelease'] if '-' in tag else []
    result = gh('release', 'create', tag, '--verify-tag', '--draft',
                '--generate-notes', '--title', f'Viff {tag}', *flags)
    if result.returncode:
        print('Create response uncertain; checking release state before retrying.', flush=True)
    time.sleep(5)
else:
    raise SystemExit('Could not create or retrieve draft release')

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
