# Viff application icon

The approved icon is the orange, text-free Git path on a white rounded plate.
`source.png` is the original full-bleed artwork and stays unchanged; the
generator scales it as a whole, keeps a 4% transparent safe margin on every
side, and rounds the plate with a corner radius of 20% of its side. Every
derived asset therefore has transparent corners. `icon.png` is the
1024 × 1024 runtime and documentation asset. Keep the figure's aspect ratio and
internal whitespace when displaying it, and do not flatten the alpha onto white.

Regenerate the PNG sizes, macOS ICNS, and Windows ICO from the original artwork:

```sh
go run ./cmd/brandassets
```

That one command reapplies the safe margin and rounded-plate alpha mask before
writing every asset. The application embeds `icon.png` for native window icons,
the welcome screen, and the About dialog. Both READMEs reference the same asset.
Packaging installs `icon.icns` in the macOS bundle, embeds `icon.ico` in the
Windows executable and installer, and installs the PNG sizes in Linux's hicolor
icon theme.

Windows packaging adds executable resources before signing or distribution. To
brand a fresh standalone Windows build manually, run:

```sh
go run ./cmd/brandassets -exe build/viff.exe -version v0.1.0
```

Run that command only once per fresh executable; existing resources are rejected.
The repository and action icons in the UI are separate from the application logo.
