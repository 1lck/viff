"""Package the locally built binary and its licenses; used by release CI."""
import pathlib
import subprocess
import sys
import zipfile

root = pathlib.Path(__file__).resolve().parent.parent
version = sys.argv[1]
goos = subprocess.check_output(["go", "env", "GOOS"], text=True).strip()
goarch = subprocess.check_output(["go", "env", "GOARCH"], text=True).strip()
binary = root / "build" / ("viff.exe" if goos == "windows" else "viff")
if not binary.is_file():
    raise SystemExit(f"Missing built binary: {binary}")
name = f"viff-{version}-{goos}-{goarch}"
dest = root / "dist"
dest.mkdir(exist_ok=True)
with zipfile.ZipFile(dest / f"{name}.zip", "w", zipfile.ZIP_DEFLATED) as archive:
    archive.write(binary, f"{name}/{binary.name}")
    for file in ["README.md", "README.zh-CN.md", "LICENSE", "THIRD_PARTY_NOTICES.md"]:
        archive.write(root / file, f"{name}/{file}")
    for file in sorted((root / "licenses").iterdir()):
        if file.is_file():
            archive.write(file, f"{name}/licenses/{file.name}")
    for file in ["LICENSE", "NOTICE.md"]:
        archive.write(root / "assets/icons/intellij" / file,
                      f"{name}/assets/icons/intellij/{file}")
print(dest / f"{name}.zip")
