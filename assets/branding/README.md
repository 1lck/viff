# Viff application icon

The approved icon is the orange, text-free Git path on a white square. Keep its
aspect ratio and whitespace when displaying it. `source.png` is the original
approved artwork; `icon.png` is the 1024 × 1024 runtime and documentation asset.

Regenerate the PNG sizes, macOS ICNS, and Windows ICO from the original artwork:

```sh
go run ./cmd/brandassets
```

The application embeds `icon.png` for native window icons, the welcome screen,
and the About dialog. Both READMEs reference the same asset. Packaging installs
`icon.icns` in the macOS bundle, embeds `icon.ico` in the Windows executable and
installer, and installs the PNG sizes in Linux's hicolor icon theme.

Windows packaging adds executable resources before signing or distribution. To
brand a fresh standalone Windows build manually, run:

```sh
go run ./cmd/brandassets -exe build/viff.exe -version v0.1.0
```

Run that command only once per fresh executable; existing resources are rejected.
The repository and action icons in the UI are separate from the application logo.
