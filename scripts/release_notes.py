"""Validate Viff's deliberately small bilingual release-note format."""
import argparse
from pathlib import Path
import re

NUMBER = r'(?:0|[1-9][0-9]*)'
IDENT = r'(?:0|[1-9][0-9]*|[0-9A-Za-z-]*[A-Za-z-][0-9A-Za-z-]*)'
VERSION = re.compile(rf'v{NUMBER}\.{NUMBER}\.{NUMBER}(?:-{IDENT}(?:\.{IDENT})*)?\Z')
HEADINGS = [
    '## 中文', '### 下载', '### 重点更新', '### 升级说明', '### 兼容性与已知问题',
    '## English', '### Downloads', '### Highlights', '### Upgrade instructions',
    '### Compatibility and known issues', '### 🙌 感谢贡献者', '### 🙌 Contributors',
]
CATEGORIES = [
    ('#### ✨ 新功能', '#### ✨ New features'),
    ('#### ⚡ 改进', '#### ⚡ Improvements'),
    ('#### 🛠 修复', '#### 🛠 Fixes'),
]
ARTIFACTS = ('darwin-arm64.dmg', 'windows-amd64-setup.exe', 'linux-amd64.deb', 'linux-amd64.tar.gz')


def visible_markdown(text):
    """Ignore comments and fenced/indented code, where examples are not content."""
    text = re.sub(r'<!--.*?-->', '', text, flags=re.S)
    lines, fence = [], None
    for line in text.splitlines():
        marker = re.match(r'^ {0,3}(`{3,}|~{3,})(.*)$', line)
        if fence:
            if marker and marker[1][0] == fence[0] and len(marker[1]) >= len(fence) and not marker[2].strip():
                fence = None
            continue
        if marker:
            fence = marker[1]
            continue
        if line.startswith(('    ', '\t')):
            continue
        lines.append(line)
    return '\n'.join(lines)


def links(text):
    # Only actual inline Markdown links count, not URLs in inline-code examples.
    text = re.sub(r'(`+).*?\1', '', text, flags=re.S)
    return set(re.findall(r'(?<!!)\[[^\]\n]+\]\(<?(https://[^\s)>]+)>?(?:\s+"[^"]*")?\)', text))


def has_content(text):
    text = re.sub(r'^ {0,3}#{1,6} .*$', '', text, flags=re.M)
    return bool(re.search(r'[\w\u4e00-\u9fff]', text))


def load_notes(path, tag, repo='1lck/viff'):
    path = Path(path)
    if not VERSION.fullmatch(tag) or path.name != f'{tag}.md':
        raise ValueError('Release tag must be valid semver and match the notes filename')
    if not re.fullmatch(r'[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+', repo):
        raise ValueError('Repository must be owner/name')
    try:
        original = path.read_text(encoding='utf-8')
    except OSError as error:
        raise ValueError(f'Cannot read release notes: {path}') from error
    if not original.strip() or re.search(r'\{\{.*?\}\}|\b(?:TODO|TBD)\b', original, flags=re.S | re.I):
        raise ValueError('Release notes are empty or contain placeholders')
    text = visible_markdown(original)
    headings = list(re.finditer(r'^ {0,3}(#{1,6} [^\n]+?)\s*$', text, re.M))
    positions = {}
    for match in headings:
        name = match[1]
        if name in positions:
            raise ValueError(f'Duplicate heading: {name}')
        positions[name] = match
    required = [positions.get(name) for name in HEADINGS]
    if any(match is None for match in required):
        raise ValueError('Missing required bilingual release-note heading')
    if [match.start() for match in required] != sorted(match.start() for match in required):
        raise ValueError('Release-note headings are out of order')

    def section(name):
        match = positions[name]
        level = len(name.split(' ', 1)[0])
        end = next((m.start() for m in headings if m.start() > match.start()
                    and len(m[1].split(' ', 1)[0]) <= level), len(text))
        return text[match.end():end]

    for name in HEADINGS:
        match = positions[name]
        end = next((m.start() for m in headings if m.start() > match.start()), len(text))
        if not has_content(text[match.end():end]):
            raise ValueError(f'Empty section: {name}')
    selected = []
    for language, highlight in enumerate(('### 重点更新', '### Highlights')):
        names = [pair[language] for pair in CATEGORIES if pair[language] in positions]
        if not names:
            raise ValueError(f'At least one change category is required in {highlight}')
        highlight_text = section(highlight)
        if any(name not in highlight_text or not has_content(section(name)) for name in names):
            raise ValueError(f'Empty or misplaced change category in {highlight}')
        if [positions[name].start() for name in names] != sorted(positions[name].start() for name in names):
            raise ValueError('Change categories are out of order')
        selected.append([i for i, pair in enumerate(CATEGORIES) if pair[language] in positions])
    if selected[0] != selected[1]:
        raise ValueError('Chinese and English change categories must match')

    base = f'https://github.com/{repo}'
    download_base = f'{base}/releases/download/{tag}'
    expected = {base, f'{base}/releases/tag/{tag}', f'{download_base}/SHA256SUMS'}
    expected.update(f'{download_base}/viff-{tag}-{suffix}' for suffix in ARTIFACTS)
    previous = []
    for download, language in (('### 下载', '## 中文'), ('### Downloads', '## English')):
        if not expected.issubset(links(section(download))):
            raise ValueError(f'Missing or incorrect release download/project links: {download}')
        comparisons = [url.removeprefix(f'{base}/compare/') for url in links(section(language))
                       if url.startswith(f'{base}/compare/')]
        if len(comparisons) != 1 or comparisons[0].count('...') != 1:
            raise ValueError(f'Exactly one comparison link is required: {language}')
        before, after = comparisons[0].split('...')
        if after != tag or before == tag or not VERSION.fullmatch(before):
            raise ValueError(f'Incorrect comparison versions: {language}')
        previous.append(before)
    if previous[0] != previous[1]:
        raise ValueError('Comparison baseline must match in both languages')
    profiles = []
    for name in HEADINGS[-2:]:
        profiles.append({url.rstrip('/') for url in links(section(name))
                         if re.fullmatch(r'https://github\.com/[A-Za-z0-9][A-Za-z0-9-]*/?', url)})
    if not profiles[0] or profiles[0] != profiles[1]:
        raise ValueError('Contributor profile links must be nonempty and match in both languages')
    return original


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('path', nargs='?', type=Path)
    parser.add_argument('--tag')
    parser.add_argument('--repo', default='1lck/viff')
    args = parser.parse_args()
    paths = [args.path] if args.path else sorted(Path('docs/releases').glob('v*.md'))
    if not paths:
        parser.error('No versioned release notes found in docs/releases')
    if args.tag and not args.path:
        parser.error('--tag requires a notes path')
    for path in paths:
        try:
            load_notes(path, args.tag or path.stem, args.repo)
        except ValueError as error:
            parser.error(str(error))
        print(f'Validated {path}')


if __name__ == '__main__':
    main()
