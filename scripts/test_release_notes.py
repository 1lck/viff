"""Release-note validation against malformed publication inputs."""
from pathlib import Path
import tempfile
import unittest

from release_notes import ARTIFACTS, load_notes


def notes(tag='v0.1.0-alpha.2'):
    base = 'https://github.com/1lck/viff'
    downloads = '\n'.join(f'- [Download]({url})' for url in [
        base, f'{base}/releases/tag/{tag}',
        *[f'{base}/releases/download/{tag}/viff-{tag}-{suffix}' for suffix in ARTIFACTS],
        f'{base}/releases/download/{tag}/SHA256SUMS',
    ])
    compare = f'[Compare]({base}/compare/v0.0.9...{tag})'
    return f'''# Viff {tag}

## 中文

轻量的 Git 客户端。

### 下载
{downloads}

### 重点更新
三个平台安装包已就绪。
#### ✨ 新功能
- 三个平台安装包。
{compare}

### 升级说明
安装新版。
### 兼容性与已知问题
未签名。

## English

A lightweight Git client.

### Downloads
{downloads}

### Highlights
Three-platform installers are ready.
#### ✨ New features
- Three-platform installers.
{compare}

### Upgrade instructions
Install the update.
### Compatibility and known issues
Unsigned.

### 🙌 感谢贡献者
[@1lck](https://github.com/1lck)
### 🙌 Contributors
[@1lck](https://github.com/1lck)
'''


class ReleaseNotesTests(unittest.TestCase):
    def validate(self, content, tag='v0.1.0-alpha.2', filename=None):
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / (filename or f'{tag}.md')
            path.write_text(content, encoding='utf-8')
            return load_notes(path, tag)

    def test_valid_stable_and_prerelease_preserve_original(self):
        for tag in ('v0.1.0', 'v0.1.0-alpha.2'):
            with self.subTest(tag=tag):
                self.assertEqual(self.validate(notes(tag), tag), notes(tag))

    def test_missing_file(self):
        with tempfile.TemporaryDirectory() as directory:
            with self.assertRaisesRegex(ValueError, 'Cannot read'):
                load_notes(Path(directory) / 'v1.0.0.md', 'v1.0.0')

    def test_wrong_tag_and_invalid_semver(self):
        for tag, filename in [('v0.1.0', 'v0.1.1.md'), ('v01.1.0', 'v01.1.0.md'),
                              ('v1.0.0-01', 'v1.0.0-01.md'), ('v1.0.0+build', 'v1.0.0+build.md')]:
            with self.subTest(tag=tag), self.assertRaises(ValueError):
                self.validate(notes(tag), tag, filename)

    def test_placeholders_even_inside_comments_are_rejected(self):
        for placeholder in ('TODO', 'TBD', '{{version}}'):
            with self.subTest(placeholder=placeholder), self.assertRaisesRegex(ValueError, 'placeholder'):
                self.validate(notes() + f'\n<!-- {placeholder} -->')

    def test_missing_language(self):
        with self.assertRaisesRegex(ValueError, 'Missing required'):
            self.validate(notes().replace('## English', '## Other'))

    def test_wrong_version_in_download_link(self):
        with self.assertRaisesRegex(ValueError, 'download'):
            self.validate(notes().replace('/download/v0.1.0-alpha.2/', '/download/v0.1.0-alpha.1/', 1))

    def test_required_empty_section(self):
        with self.assertRaisesRegex(ValueError, 'Empty section'):
            self.validate(notes().replace('Install the update.', '<!-- text does not count -->'))

    def test_language_summary_and_highlight_overview_cannot_be_empty(self):
        for content in ('轻量的 Git 客户端。', 'A lightweight Git client.',
                        '三个平台安装包已就绪。', 'Three-platform installers are ready.'):
            with self.subTest(content=content), self.assertRaisesRegex(ValueError, 'Empty section'):
                self.validate(notes().replace(content, ''))

    def test_categories_must_be_present_mirrored_and_nonempty(self):
        replacements = [
            ('#### ✨ New features', '#### ⚡ Improvements'),
            ('#### ✨ New features', ''),
            ('- Three-platform installers.\n[Compare](https://github.com/1lck/viff/compare/v0.0.9...v0.1.0-alpha.2)', ''),
        ]
        for old, new in replacements:
            with self.subTest(old=old), self.assertRaises(ValueError):
                self.validate(notes().replace(old, new))

    def test_comments_and_code_cannot_supply_required_headings(self):
        for wrapper in ('<!--\n{}\n-->', '```markdown\n{}\n```', '~~~\n{}\n~~~'):
            with self.subTest(wrapper=wrapper), self.assertRaisesRegex(ValueError, 'Missing required'):
                self.validate(notes().replace('### Downloads', wrapper.format('### Downloads')))

    def test_ordered_headings(self):
        with self.assertRaisesRegex(ValueError, 'out of order'):
            self.validate(notes().replace('### 升级说明', '### TEMP').replace(
                '### 兼容性与已知问题', '### 升级说明').replace('### TEMP', '### 兼容性与已知问题'))

    def test_comparison_target_baseline_and_self_comparison(self):
        for replacement in ('v0.0.9...v0.1.0', 'v0.1.0-alpha.2...v0.1.0-alpha.2',
                            'v0.0.8...v0.1.0-alpha.2'):
            with self.subTest(replacement=replacement), self.assertRaises(ValueError):
                self.validate(notes().replace('v0.0.9...v0.1.0-alpha.2', replacement, 1))

    def test_contributors_must_match(self):
        with self.assertRaisesRegex(ValueError, 'Contributor'):
            self.validate(notes().replace('](https://github.com/1lck)', '](https://github.com/someone)', 1))


if __name__ == '__main__':
    unittest.main()
