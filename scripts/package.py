"""Build native distribution packages from build/viff; never launch the app.

Run on each target OS: python scripts/package.py v0.1.0 [--smoke]
--smoke additionally exercises install/uninstall on disposable CI runners only.
--arch and --binary package a cross-compiled build, such as the macOS Intel DMG
built on the Apple Silicon runner.
"""
import argparse
import os
from pathlib import Path
import plistlib
import re
import shutil
import subprocess
import tarfile
import tempfile

ROOT = Path(__file__).resolve().parent.parent


def run(*args, **kwargs):
    return subprocess.run([str(arg) for arg in args], check=True, **kwargs)


def copy_notices(destination):
    destination.mkdir(parents=True, exist_ok=True)
    for name in ("README.md", "README.zh-CN.md", "LICENSE", "THIRD_PARTY_NOTICES.md",
                 "CONTRIBUTING.md", "SECURITY.md"):
        shutil.copy2(ROOT / name, destination / name)
    for name in ("docs", "licenses"):
        shutil.copytree(ROOT / name, destination / name)
    shutil.copytree(ROOT / "assets/branding", destination / "assets/branding",
                    ignore=shutil.ignore_patterns("*.go", "source.png"))
    icons = destination / "assets/icons/intellij"
    icons.mkdir(parents=True)
    for name in ("LICENSE", "NOTICE.md"):
        shutil.copy2(ROOT / "assets/icons/intellij" / name, icons / name)


def require_notices(directory):
    for name in ("LICENSE", "THIRD_PARTY_NOTICES.md", "assets/icons/intellij/LICENSE",
                 "assets/icons/intellij/NOTICE.md"):
        if not (directory / name).is_file():
            raise RuntimeError(f"Missing packaged license: {directory / name}")
    if not any((directory / "licenses").iterdir()):
        raise RuntimeError("Missing dependency licenses")


def package_mac(binary, name, version, arch, work, dest):
    # lipo reports Mach-O slice names, which differ from Go's GOARCH for Intel.
    expected = {"amd64": "x86_64"}.get(arch, arch)
    built = subprocess.check_output(["lipo", "-archs", str(binary)], text=True).split()
    if built != [expected]:
        raise RuntimeError(f"Binary architecture {built} does not match {expected}")
    stage = work / "volume"
    contents = stage / "Viff.app/Contents"
    (contents / "MacOS").mkdir(parents=True)
    shutil.copy2(binary, contents / "MacOS/viff")
    (contents / "MacOS/viff").chmod(0o755)
    copy_notices(contents / "Resources")
    shutil.copy2(ROOT / "assets/branding/icon.icns", contents / "Resources/viff.icns")
    numeric_version = version.lstrip("v").split("-", 1)[0]
    with (contents / "Info.plist").open("wb") as stream:
        plistlib.dump({
            "CFBundleName": "Viff", "CFBundleDisplayName": "Viff",
            "CFBundleIdentifier": "io.github.1lck.viff", "CFBundleExecutable": "viff",
            "CFBundleIconFile": "viff.icns",
            "CFBundlePackageType": "APPL", "CFBundleInfoDictionaryVersion": "6.0",
            "CFBundleShortVersionString": numeric_version,
            "CFBundleVersion": numeric_version, "LSMinimumSystemVersion": "13.0",
            "LSApplicationCategoryType": "public.app-category.developer-tools",
            "NSHighResolutionCapable": True,
            "NSHumanReadableCopyright": "Copyright Viff contributors. MIT License.",
        }, stream)
    # Ad-hoc signing lets arm64 load the completed bundle. This is NOT Developer
    # ID signing or notarization; public documentation states that distinction.
    run("codesign", "--force", "--sign", "-", contents.parent)
    run("codesign", "--verify", "--deep", "--strict", contents.parent)
    (stage / "Applications").symlink_to("/Applications")
    output = dest / f"{name}.dmg"
    run("hdiutil", "create", "-ov", "-volname", "Viff", "-srcfolder", stage,
        "-format", "UDZO", output)
    mount = work / "mount"
    mount.mkdir()
    run("hdiutil", "attach", "-readonly", "-nobrowse", "-mountpoint", mount, output)
    try:
        require_notices(mount / "Viff.app/Contents/Resources")
        if (mount / "Viff.app/Contents/MacOS/viff").read_bytes() != binary.read_bytes():
            # codesign may update the executable's ad-hoc signature.
            run("codesign", "--verify", "--deep", "--strict", mount / "Viff.app")
        if not (mount / "Applications").is_symlink():
            raise RuntimeError("DMG has no Applications shortcut")
    finally:
        run("hdiutil", "detach", mount)
    return [output]


def nsis_string(value):
    return str(value).replace("$", "$$").replace('"', '$\\"')


def package_windows(binary, name, version, work, dest, smoke):
    payload = work / "payload"
    copy_notices(payload)
    shutil.copy2(binary, payload / "viff.exe")
    binary = payload / "viff.exe"
    run("go", "run", "./cmd/brandassets", "-exe", binary, "-version", version, cwd=ROOT)
    output = dest / f"{name}-setup.exe"
    script = work / "installer.nsi"
    # A per-user install avoids UAC and keeps all registry writes in HKCU.
    script.write_text(f'''Unicode true
!include "MUI2.nsh"
Name "Viff"
OutFile "{nsis_string(output)}"
InstallDir "$LOCALAPPDATA\\Programs\\Viff"
InstallDirRegKey HKCU "Software\\Viff" "InstallDir"
RequestExecutionLevel user
SetCompressor /SOLID lzma
!define MUI_ICON "{nsis_string(ROOT / 'assets/branding/icon.ico')}"
!define MUI_UNICON "{nsis_string(ROOT / 'assets/branding/icon.ico')}"
!define MUI_ABORTWARNING
!insertmacro MUI_PAGE_WELCOME
!insertmacro MUI_PAGE_LICENSE "{nsis_string(ROOT / 'LICENSE')}"
!insertmacro MUI_PAGE_DIRECTORY
!insertmacro MUI_PAGE_INSTFILES
!insertmacro MUI_PAGE_FINISH
!insertmacro MUI_UNPAGE_CONFIRM
!insertmacro MUI_UNPAGE_INSTFILES
!insertmacro MUI_LANGUAGE "English"
Section "Viff" SEC_MAIN
  SetShellVarContext current
  SetOutPath "$INSTDIR"
  File /r "{nsis_string(payload)}\\*"
  WriteUninstaller "$INSTDIR\\Uninstall.exe"
  CreateDirectory "$SMPROGRAMS\\Viff"
  CreateShortcut "$SMPROGRAMS\\Viff\\Viff.lnk" "$INSTDIR\\viff.exe"
  CreateShortcut "$SMPROGRAMS\\Viff\\Uninstall.lnk" "$INSTDIR\\Uninstall.exe"
  WriteRegStr HKCU "Software\\Viff" "InstallDir" "$INSTDIR"
  WriteRegStr HKCU "Software\\Microsoft\\Windows\\CurrentVersion\\Uninstall\\Viff" "DisplayIcon" '$\\"$INSTDIR\\viff.exe$\\",0'
  WriteRegStr HKCU "Software\\Microsoft\\Windows\\CurrentVersion\\Uninstall\\Viff" "DisplayName" "Viff"
  WriteRegStr HKCU "Software\\Microsoft\\Windows\\CurrentVersion\\Uninstall\\Viff" "DisplayVersion" "{version}"
  WriteRegStr HKCU "Software\\Microsoft\\Windows\\CurrentVersion\\Uninstall\\Viff" "Publisher" "Viff contributors"
  WriteRegStr HKCU "Software\\Microsoft\\Windows\\CurrentVersion\\Uninstall\\Viff" "UninstallString" '$\\"$INSTDIR\\Uninstall.exe$\\"'
  WriteRegStr HKCU "Software\\Microsoft\\Windows\\CurrentVersion\\Uninstall\\Viff" "QuietUninstallString" '$\\"$INSTDIR\\Uninstall.exe$\\" /S'
  WriteRegDWORD HKCU "Software\\Microsoft\\Windows\\CurrentVersion\\Uninstall\\Viff" "NoModify" 1
  WriteRegDWORD HKCU "Software\\Microsoft\\Windows\\CurrentVersion\\Uninstall\\Viff" "NoRepair" 1
SectionEnd
Section "Uninstall"
  SetShellVarContext current
  Delete "$SMPROGRAMS\\Viff\\Viff.lnk"
  Delete "$SMPROGRAMS\\Viff\\Uninstall.lnk"
  RMDir "$SMPROGRAMS\\Viff"
  DeleteRegKey HKCU "Software\\Microsoft\\Windows\\CurrentVersion\\Uninstall\\Viff"
  DeleteRegKey HKCU "Software\\Viff"
  Delete "$INSTDIR\\viff.exe"
  Delete "$INSTDIR\\Uninstall.exe"
''', encoding="utf-8")
    # Remove only installed files, preserving any user-created files.
    with script.open("a", encoding="utf-8") as stream:
        for file in sorted(payload.rglob("*")):
            if file.is_file() and file.name != "viff.exe":
                relative = str(file.relative_to(payload)).replace("/", "\\")
                stream.write(f'  Delete "$INSTDIR\\{nsis_string(relative)}"\n')
        for directory in sorted((p for p in payload.rglob("*") if p.is_dir()),
                                key=lambda p: len(p.parts), reverse=True):
            relative = str(directory.relative_to(payload)).replace("/", "\\")
            stream.write(f'  RMDir "$INSTDIR\\{nsis_string(relative)}"\n')
        stream.write('  RMDir "$INSTDIR"\nSectionEnd\n')
    makensis = shutil.which("makensis")
    if not makensis:
        candidate = Path(os.environ.get("ProgramFiles(x86)", "C:/Program Files (x86)")) / "NSIS/makensis.exe"
        if candidate.is_file():
            makensis = str(candidate)
    if not makensis:
        raise RuntimeError("NSIS is required: install it and add makensis to PATH")
    run(makensis, "-V3", script)
    if output.read_bytes()[:2] != b"MZ":
        raise RuntimeError("Installer is not a Windows executable")
    if smoke:
        import winreg

        uninstall_key = r"Software\Microsoft\Windows\CurrentVersion\Uninstall\Viff"
        install = work / "installed"
        run(output, "/S", f"/D={install}")
        try:
            require_notices(install)
            with winreg.OpenKey(winreg.HKEY_CURRENT_USER, uninstall_key) as key:
                if winreg.QueryValueEx(key, "DisplayVersion")[0] != version:
                    raise RuntimeError("Installer registration has the wrong version")
            if (install / "viff.exe").read_bytes() != binary.read_bytes():
                raise RuntimeError("Installed binary differs from build")
        finally:
            uninstaller = install / "Uninstall.exe"
            if uninstaller.exists():
                # _?= runs synchronously instead of copying itself to TEMP.
                run(uninstaller, "/S", f"_?={install}")
        if (install / "viff.exe").exists():
            raise RuntimeError("Uninstall did not remove the application")
        try:
            with winreg.OpenKey(winreg.HKEY_CURRENT_USER, uninstall_key):
                raise RuntimeError("Uninstall left its registration behind")
        except FileNotFoundError:
            pass
    return [output]


def package_linux(binary, name, version, arch, work, dest, smoke):
    portable = work / name
    copy_notices(portable)
    shutil.copy2(binary, portable / "viff")
    (portable / "viff").chmod(0o755)
    tar_path = dest / f"{name}.tar.gz"
    with tarfile.open(tar_path, "w:gz") as archive:
        archive.add(portable, arcname=name)
    with tarfile.open(tar_path, "r:gz") as archive:
        executable = archive.getmember(f"{name}/viff")
        if not executable.isfile() or not executable.mode & 0o111:
            raise RuntimeError("Portable archive has no executable")
        for relative in ("LICENSE", "THIRD_PARTY_NOTICES.md", "assets/icons/intellij/LICENSE"):
            if not archive.getmember(f"{name}/{relative}").isfile():
                raise RuntimeError(f"Portable archive is missing {relative}")
    deb = work / "deb"
    (deb / "usr/bin").mkdir(parents=True)
    shutil.copy2(portable / "viff", deb / "usr/bin/viff")
    copy_notices(deb / "usr/share/doc/viff")
    for size in (16, 32, 48, 64, 128, 256, 512):
        icon_dir = deb / f"usr/share/icons/hicolor/{size}x{size}/apps"
        icon_dir.mkdir(parents=True)
        shutil.copy2(ROOT / f"assets/branding/icon-{size}.png", icon_dir / "viff.png")
    applications = deb / "usr/share/applications"
    applications.mkdir(parents=True)
    (applications / "viff.desktop").write_text(
        "[Desktop Entry]\nType=Application\nName=Viff\n"
        "Comment=Lightweight native Git client\nExec=viff\n"
        "Icon=viff\nStartupWMClass=viff\nTerminal=false\nCategories=Development;RevisionControl;\n", encoding="utf-8")
    (deb / "DEBIAN").mkdir()
    deb_version = version.lstrip("v").replace("-", "~", 1)
    (deb / "DEBIAN/control").write_text(
        f"Package: viff\nVersion: {deb_version}\nArchitecture: {arch}\n"
        "Maintainer: Viff contributors <viff@users.noreply.github.com>\n"
        "Section: devel\nPriority: optional\n"
        "Homepage: https://github.com/1lck/viff\n"
        "Depends: git, libvulkan1, libxcb1, libcairo2, libfontconfig1, libfreetype6, libharfbuzz0b, libpango-1.0-0, libpangocairo-1.0-0\n"
        "Description: Lightweight native Git desktop client\n"
        " A Go and Viva Git client without an Electron or WebView runtime.\n",
        encoding="utf-8")
    deb_path = dest / f"{name}.deb"
    run("dpkg-deb", "--build", "--root-owner-group", deb, deb_path)
    extracted = work / "deb-extracted"
    run("dpkg-deb", "--extract", deb_path, extracted)
    require_notices(extracted / "usr/share/doc/viff")
    if (extracted / "usr/bin/viff").read_bytes() != binary.read_bytes():
        raise RuntimeError("DEB binary differs from build")
    if smoke:
        try:
            run("sudo", "apt-get", "install", "-y", deb_path)
            require_notices(Path("/usr/share/doc/viff"))
            if Path("/usr/bin/viff").read_bytes() != binary.read_bytes():
                raise RuntimeError("Installed DEB binary differs from build")
        finally:
            run("sudo", "dpkg", "--remove", "viff")
        if Path("/usr/bin/viff").exists():
            raise RuntimeError("DEB uninstall left the binary behind")
    return [deb_path, tar_path]


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("version")
    parser.add_argument("--smoke", action="store_true")
    parser.add_argument("--arch", help="target GOARCH; defaults to the host GOARCH")
    parser.add_argument("--binary", type=Path,
                        help="built binary; defaults to build/viff (or build/viff.exe)")
    args = parser.parse_args()
    if not re.fullmatch(r"v?\d+\.\d+\.\d+(?:-[A-Za-z0-9.-]+)?", args.version):
        parser.error("version must be a semantic version, optionally prefixed with v")
    if args.smoke and os.environ.get("GITHUB_ACTIONS") != "true":
        parser.error("--smoke requires a disposable GitHub Actions runner")
    goos = subprocess.check_output(["go", "env", "GOOS"], text=True).strip()
    arch = args.arch or subprocess.check_output(["go", "env", "GOARCH"], text=True).strip()
    if not re.fullmatch(r"[a-z0-9]+", arch):
        parser.error(f"invalid architecture: {arch}")
    binary = args.binary or ROOT / "build" / ("viff.exe" if goos == "windows" else "viff")
    if not binary.is_absolute():
        binary = ROOT / binary
    if not binary.is_file():
        raise SystemExit(f"Missing built binary: {binary}")
    dest = ROOT / "dist"
    dest.mkdir(exist_ok=True)
    name = f"viff-{args.version}-{goos}-{arch}"
    with tempfile.TemporaryDirectory(prefix="viff-package-") as temporary:
        work = Path(temporary)
        if goos == "darwin":
            outputs = package_mac(binary, name, args.version, arch, work, dest)
        elif goos == "windows":
            outputs = package_windows(binary, name, args.version, work, dest, args.smoke)
        elif goos == "linux":
            outputs = package_linux(binary, name, args.version, arch, work, dest, args.smoke)
        else:
            raise SystemExit(f"Unsupported target: {goos}")
    for output in outputs:
        print(f"Verified package: {output} ({output.stat().st_size:,} bytes)")


if __name__ == "__main__":
    main()
