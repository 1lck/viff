// Command brandassets derives platform icons from the approved artwork or embeds
// the Windows icon into a freshly built executable before packaging/signing.
package main

import (
	"flag"
	"image/png"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/vivaui/viva/tools/appbundle"
	"github.com/vivaui/viva/tools/winpkg"
	"github.com/vivaui/viva/tools/winres"
)

func must(err error) {
	if err != nil {
		panic(err)
	}
}
func main() {
	exe := flag.String("exe", "", "Windows executable to brand")
	version := flag.String("version", "0.0.0", "release version")
	flag.Parse()
	dir := "assets/branding"
	if *exe != "" {
		binary, err := os.ReadFile(*exe)
		must(err)
		icon, err := os.ReadFile(filepath.Join(dir, "icon.ico"))
		must(err)
		result, err := winres.Patch(binary, icon, winres.Info{Name: "Viff", Description: "Viff — native Git client", Version: strings.Split(strings.TrimPrefix(*version, "v"), "-")[0], Publisher: "Viff contributors", File: "viff.exe"})
		must(err)
		must(os.WriteFile(*exe, result, 0755))
		return
	}
	file, err := os.Open(filepath.Join(dir, "source.png"))
	must(err)
	source, err := png.Decode(file)
	must(err)
	must(file.Close())
	square := appbundle.Downscale(source, 1024)
	for _, size := range []int{16, 32, 48, 64, 128, 256, 512, 1024} {
		name := filepath.Join(dir, "icon-"+strconv.Itoa(size)+".png")
		if size == 1024 {
			name = filepath.Join(dir, "icon.png")
		}
		out, err := os.Create(name)
		must(err)
		must(png.Encode(out, appbundle.Downscale(square, size)))
		must(out.Close())
	}
	icns, _, err := appbundle.EncodeICNS(square)
	must(err)
	must(os.WriteFile(filepath.Join(dir, "icon.icns"), icns, 0644))
	ico, _, err := winpkg.EncodeICO(square)
	must(err)
	must(os.WriteFile(filepath.Join(dir, "icon.ico"), ico, 0644))
}
