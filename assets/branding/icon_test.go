package branding_test

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	"image/png"
	"os"
	"testing"
)

const runtimeIconSize = 1024

func TestRuntimeIconPNG(t *testing.T) {
	img := loadPNG(t, "icon.png")
	checkFrame(t, "icon.png", img, runtimeIconSize)

	alpha := alphaBounds(img)
	if alpha.Empty() {
		t.Fatal("icon.png is fully transparent")
	}
	margins := []int{alpha.Min.X, alpha.Min.Y, runtimeIconSize - alpha.Max.X, runtimeIconSize - alpha.Max.Y}
	for _, margin := range margins {
		if margin < runtimeIconSize*3/100 || margin > runtimeIconSize*5/100 {
			t.Errorf("icon.png safe margin = %d px, want 3%%–5%% of the canvas", margin)
		}
	}
	if alphaAt(t, img, alpha.Min.X+1, alpha.Min.Y+1) != 0 {
		t.Error("icon.png corner is square; the plate's alpha must be rounded")
	}
	if alphaAt(t, img, runtimeIconSize/2, alpha.Min.Y) == 0 {
		t.Error("icon.png misses the plate just inside the top margin")
	}
}

func TestGeneratedPNGSet(t *testing.T) {
	for _, size := range []int{16, 32, 48, 64, 128, 256, 512} {
		name := fmt.Sprintf("icon-%d.png", size)
		img := loadPNG(t, name)
		checkFrame(t, name, img, size)
		if size >= 32 {
			alpha := alphaBounds(img)
			if alpha.Min.X == 0 || alpha.Min.Y == 0 || alpha.Max.X == size || alpha.Max.Y == size {
				t.Errorf("%s touches the canvas edge; the safe margin is missing", name)
			}
		}
	}
}

func TestWindowsICO(t *testing.T) {
	data, err := os.ReadFile("icon.ico")
	if err != nil {
		t.Fatal(err)
	}
	if len(data) < 6 {
		t.Fatal("icon.ico is shorter than its header")
	}
	count := int(binary.LittleEndian.Uint16(data[4:]))
	if count < 14 {
		t.Fatalf("icon.ico has %d frames, want at least the 14 frames (16–256 px)", count)
	}
	if len(data) < 6+16*count {
		t.Fatal("icon.ico directory is truncated")
	}

	frames := make(map[int]image.Image, count)
	for index := 0; index < count; index++ {
		entry := data[6+16*index:]
		width, height := int(entry[0]), int(entry[1])
		if width == 0 {
			width = 256
		}
		if height == 0 {
			height = 256
		}
		if width != height {
			t.Fatalf("icon.ico frame %d is %dx%d, want a square", index, width, height)
		}
		if planes, bits := binary.LittleEndian.Uint16(entry[4:]), binary.LittleEndian.Uint16(entry[6:]); planes != 1 || bits != 32 {
			t.Fatalf("icon.ico frame %d is %d planes x %d bits, want 1 x 32", index, planes, bits)
		}
		size := int(binary.LittleEndian.Uint32(entry[8:]))
		offset := int(binary.LittleEndian.Uint32(entry[12:]))
		if offset < 6+16*count || size <= 0 || offset+size > len(data) {
			t.Fatalf("icon.ico frame %d lies outside the file", index)
		}
		payload := data[offset : offset+size]
		if !bytes.HasPrefix(payload, []byte("\x89PNG\r\n\x1a\n")) {
			t.Fatalf("icon.ico frame %d is not a PNG", index)
		}
		img, err := png.Decode(bytes.NewReader(payload))
		if err != nil {
			t.Fatalf("icon.ico frame %d: %v", index, err)
		}
		if img.Bounds().Dx() != width || img.Bounds().Dy() != height {
			t.Fatalf("icon.ico frame %d decodes as %v, want %dx%d", index, img.Bounds(), width, height)
		}
		checkFrame(t, fmt.Sprintf("icon.ico %dpx", width), img, width)
		frames[width] = img
	}

	for _, want := range []int{16, 32, 48, 256} {
		if frames[want] == nil {
			t.Errorf("icon.ico is missing its %d px frame", want)
		}
	}
}

func TestMacOSICNS(t *testing.T) {
	data, err := os.ReadFile("icon.icns")
	if err != nil {
		t.Fatal(err)
	}
	if len(data) < 8 || string(data[:4]) != "icns" {
		t.Fatal("icon.icns does not start with the ICNS magic")
	}
	if declared := int(binary.BigEndian.Uint32(data[4:])); declared != len(data) {
		t.Fatalf("icon.icns declares %d bytes, file has %d", declared, len(data))
	}

	seen := map[int]bool{}
	for offset := 8; offset < len(data); {
		if offset+8 > len(data) {
			t.Fatalf("icon.icns entry at %d is truncated", offset)
		}
		kind := string(data[offset : offset+4])
		length := int(binary.BigEndian.Uint32(data[offset+4:]))
		if length < 8 || offset+length > len(data) {
			t.Fatalf("icon.icns entry %q at %d has invalid length %d", kind, offset, length)
		}
		img, err := png.Decode(bytes.NewReader(data[offset+8 : offset+length]))
		if err != nil {
			t.Fatalf("icon.icns entry %q: %v", kind, err)
		}
		size := img.Bounds().Dx()
		if size != img.Bounds().Dy() {
			t.Fatalf("icon.icns entry %q is not square: %v", kind, img.Bounds())
		}
		if size == 32 || size == 256 || size == runtimeIconSize {
			checkFrame(t, "icon.icns "+kind, img, size)
			seen[size] = true
		}
		offset += length
	}
	for _, want := range []int{32, 256, runtimeIconSize} {
		if !seen[want] {
			t.Errorf("icon.icns is missing a %d px representation", want)
		}
	}
}

func loadPNG(t *testing.T, name string) image.Image {
	t.Helper()
	data, err := os.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	return img
}

func alphaAt(t *testing.T, img image.Image, x, y int) uint8 {
	t.Helper()
	_, _, _, a := img.At(x, y).RGBA()
	return uint8(a >> 8)
}

func alphaBounds(img image.Image) image.Rectangle {
	bounds := img.Bounds()
	result := image.Rectangle{}
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			if _, _, _, a := img.At(x, y).RGBA(); a != 0 {
				result = result.Union(image.Rect(x, y, x+1, y+1))
			}
		}
	}
	return result
}

// checkFrame asserts the properties every derived icon frame must keep: the
// requested size, transparent rounded corners, an opaque plate, and the whole
// orange figure inside the canvas.
func checkFrame(t *testing.T, name string, img image.Image, size int) {
	t.Helper()
	bounds := img.Bounds()
	if bounds.Dx() != size || bounds.Dy() != size {
		t.Fatalf("%s is %v, want %dx%d", name, bounds, size, size)
	}
	for _, corner := range [][2]int{{0, 0}, {size - 1, 0}, {0, size - 1}, {size - 1, size - 1}} {
		if a := alphaAt(t, img, corner[0], corner[1]); a != 0 {
			t.Fatalf("%s corner %v alpha = %d, want 0", name, corner, a)
		}
	}
	if a := alphaAt(t, img, size/2, size/2); a != 255 {
		t.Fatalf("%s center alpha = %d, want 255", name, a)
	}

	orange := 0
	figure := image.Rectangle{}
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			r, g, b, a := img.At(x, y).RGBA()
			if r>>8 <= 180 || g>>8 >= 150 || b>>8 >= 120 || a == 0 {
				continue
			}
			orange++
			if a != 0xffff {
				t.Fatalf("%s figure pixel (%d,%d) alpha = %d, want 255", name, x, y, a>>8)
			}
			figure = figure.Union(image.Rect(x, y, x+1, y+1))
		}
	}
	if orange == 0 {
		t.Fatalf("%s has no orange figure", name)
	}
	if figure.Min.X == 0 || figure.Min.Y == 0 || figure.Max.X == size || figure.Max.Y == size {
		t.Fatalf("%s clips the orange figure at the canvas edge", name)
	}
}
