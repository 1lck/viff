// Command genicons compiles the IntelliJ SVG icons into Viva icons.
//
// Viva's icon compiler follows the Lucide dialect: one stroke width for the
// whole glyph (2 by default), strokes tinted at draw time, and a concrete
// fill turned into a coloured layer. IntelliJ icons instead use SVG defaults
// (1px strokes, butt caps), give strokes concrete colours, and mix filled and
// stroked shapes, sometimes on one element. To draw them faithfully each icon
// is split, in document order, into parts with a single paint: filled parts
// keep their colours as layers, stroked parts become tinted outlines drawn in
// the stroke's own colour and width. A glyph is the ordered list of parts.
package main

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/vivaui/viva/tools/iconcompile"
	"github.com/vivaui/viva/ui"
)

type node struct {
	Name     xml.Name
	Attrs    []xml.Attr
	Children []*node
	Text     string
}

func (n *node) attr(name string) (string, bool) {
	for _, a := range n.Attrs {
		if a.Name.Local == name {
			return a.Value, true
		}
	}
	return "", false
}

func (n *node) set(name, value string) {
	for i, a := range n.Attrs {
		if a.Name.Local == name {
			n.Attrs[i].Value = value
			return
		}
	}
	n.Attrs = append(n.Attrs, xml.Attr{Name: xml.Name{Local: name}, Value: value})
}

func (n *node) del(name string) {
	out := n.Attrs[:0]
	for _, a := range n.Attrs {
		if a.Name.Local != name {
			out = append(out, a)
		}
	}
	n.Attrs = out
}

func parse(data []byte) (*node, error) {
	dec := xml.NewDecoder(bytes.NewReader(data))
	var stack []*node
	var root *node
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			n := &node{Name: t.Name, Attrs: append([]xml.Attr(nil), t.Attr...)}
			if len(stack) > 0 {
				p := stack[len(stack)-1]
				p.Children = append(p.Children, n)
			} else {
				root = n
			}
			stack = append(stack, n)
		case xml.EndElement:
			stack = stack[:len(stack)-1]
		case xml.CharData:
			if len(stack) > 0 {
				stack[len(stack)-1].Text += string(t)
			}
		}
	}
	if root == nil {
		return nil, fmt.Errorf("empty document")
	}
	return root, nil
}

func (n *node) write(b *strings.Builder) {
	b.WriteString("<" + n.Name.Local)
	for _, a := range n.Attrs {
		if a.Name.Space == "xmlns" || a.Name.Local == "xmlns" {
			continue
		}
		name := a.Name.Local
		if a.Name.Space != "" && a.Name.Space != "http://www.w3.org/2000/svg" {
			if strings.Contains(a.Name.Space, "xlink") {
				name = "xlink:" + a.Name.Local
			}
		}
		fmt.Fprintf(b, " %s=%q", name, a.Value)
	}
	b.WriteString(">")
	b.WriteString(xmlEscape(strings.TrimSpace(n.Text)))
	for _, c := range n.Children {
		c.write(b)
	}
	b.WriteString("</" + n.Name.Local + ">")
}

func xmlEscape(s string) string {
	var b bytes.Buffer
	xml.EscapeText(&b, []byte(s))
	return b.String()
}

func (n *node) clone() *node {
	c := &node{Name: n.Name, Attrs: append([]xml.Attr(nil), n.Attrs...), Text: n.Text}
	for _, ch := range n.Children {
		c.Children = append(c.Children, ch.clone())
	}
	return c
}

var shapes = map[string]bool{"path": true, "circle": true, "rect": true, "line": true, "polyline": true, "polygon": true, "ellipse": true}

// paint is the resolved fill and stroke of a shape.
type paint struct {
	fill, stroke              string
	width, cap, join          string
	opacity, fillOp, strokeOp float64
}

// shapeRef is one shape with its ancestors' inherited paint.
type shapeRef struct {
	el *node
	p  paint
}

func collect(n *node, inherited paint, inDefs bool, out *[]shapeRef) {
	p := inherited
	if v, ok := n.attr("fill"); ok {
		p.fill = v
	}
	if v, ok := n.attr("stroke"); ok {
		p.stroke = v
	}
	if v, ok := n.attr("stroke-width"); ok {
		p.width = v
	}
	if v, ok := n.attr("stroke-linecap"); ok {
		p.cap = v
	}
	if v, ok := n.attr("stroke-linejoin"); ok {
		p.join = v
	}
	if v, ok := n.attr("opacity"); ok {
		f, _ := strconv.ParseFloat(v, 64)
		p.opacity *= f
	}
	if v, ok := n.attr("fill-opacity"); ok {
		p.fillOp, _ = strconv.ParseFloat(v, 64)
	}
	if v, ok := n.attr("stroke-opacity"); ok {
		p.strokeOp, _ = strconv.ParseFloat(v, 64)
	}
	switch n.Name.Local {
	case "defs", "clipPath", "mask", "linearGradient", "radialGradient":
		inDefs = true
	}
	if shapes[n.Name.Local] && !inDefs {
		*out = append(*out, shapeRef{n, p})
	}
	for _, c := range n.Children {
		collect(c, p, inDefs, out)
	}
}

// part is one single-paint layer of a glyph.
type part struct {
	icon  ui.Icon
	tint  ui.Color
	layer bool
}

func parseColor(s string, alpha float64) (ui.Color, bool) {
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, "#") {
		switch s {
		case "white":
			s = "#FFFFFF"
		case "black":
			s = "#000000"
		default:
			return ui.Color{}, false
		}
	}
	h := s[1:]
	if len(h) == 3 {
		h = string([]byte{h[0], h[0], h[1], h[1], h[2], h[2]})
	}
	var a uint64 = 255
	if len(h) == 8 {
		a, _ = strconv.ParseUint(h[6:], 16, 8)
		h = h[:6]
	}
	v, err := strconv.ParseUint(h, 16, 32)
	if err != nil {
		return ui.Color{}, false
	}
	c := ui.RGB24(uint32(v))
	c.Alpha = float32(float64(a) / 255 * alpha)
	return c, true
}

// compileParts splits one SVG into parts.
func compileParts(name string, data []byte) ([]part, error) {
	root, err := parse(data)
	if err != nil {
		return nil, err
	}
	var refs []shapeRef
	collect(root, paint{fill: "black", opacity: 1, fillOp: 1, strokeOp: 1}, false, &refs)
	type job struct {
		keep   map[*node]bool
		stroke bool
		p      paint
	}
	var jobs []job
	for _, r := range refs {
		hasFill := r.p.fill != "none" && r.p.fill != ""
		hasStroke := r.p.stroke != "none" && r.p.stroke != ""
		if hasFill {
			key := r.p
			if n := len(jobs); n > 0 && !jobs[n-1].stroke {
				jobs[n-1].keep[r.el] = true
			} else {
				jobs = append(jobs, job{keep: map[*node]bool{r.el: true}, p: key})
			}
		}
		if hasStroke {
			if n := len(jobs); n > 0 && jobs[n-1].stroke && samePaintStroke(jobs[n-1].p, r.p) {
				jobs[n-1].keep[r.el] = true
			} else {
				jobs = append(jobs, job{keep: map[*node]bool{r.el: true}, stroke: true, p: r.p})
			}
		}
	}
	var parts []part
	for i, j := range jobs {
		doc := root.clone()
		var prune func(orig, copy *node)
		prune = func(orig, cp *node) {
			var kept []*node
			for idx, ch := range orig.Children {
				cc := cp.Children[idx]
				if shapes[ch.Name.Local] && !isInDefs(orig) {
					if !j.keep[ch] {
						continue
					}
					if j.stroke {
						cc.set("fill", "none")
						cc.set("stroke", "currentColor")
						cc.del("stroke-width")
						cc.del("stroke-opacity")
						cc.del("opacity")
					} else {
						cc.set("stroke", "none")
					}
				} else {
					prune(ch, cc)
				}
				kept = append(kept, cc)
			}
			cp.Children = kept
		}
		prune(root, doc)
		if j.stroke {
			w := j.p.width
			if w == "" {
				w = "1"
			}
			doc.set("stroke-width", w)
			cp := j.p.cap
			if cp == "" {
				cp = "butt"
			}
			jn := j.p.join
			if jn == "" {
				jn = "miter"
			}
			doc.set("stroke-linecap", cp)
			doc.set("stroke-linejoin", jn)
			// Groups may carry a stroke colour too.
			clearGroupStroke(doc)
		} else {
			doc.set("stroke-width", "0")
		}
		var b strings.Builder
		doc.write(&b)
		icon, err := iconcompile.Compile(fmt.Sprintf("%s#%d", name, i), []byte(b.String()))
		if err != nil {
			return nil, fmt.Errorf("%s part %d: %w\n%s", name, i, err, b.String())
		}
		pt := part{icon: icon}
		if j.stroke {
			c, ok := parseColor(j.p.stroke, j.p.opacity*j.p.strokeOp)
			if !ok {
				c = ui.Color{Red: 1, Green: 1, Blue: 1, Alpha: 1}
			}
			pt.tint = c
		} else {
			pt.layer = true
			pt.tint = ui.Color{Red: 1, Green: 1, Blue: 1, Alpha: float32(j.p.opacity)}
			if len(icon.Layers) == 0 && len(icon.Commands) > 0 {
				// A currentColor fill compiles to tinted commands.
				c, ok := parseColor(j.p.fill, j.p.opacity*j.p.fillOp)
				if ok {
					pt.tint = c
				}
			}
		}
		parts = append(parts, pt)
	}
	return parts, nil
}

func isInDefs(n *node) bool {
	switch n.Name.Local {
	case "defs", "clipPath", "mask", "linearGradient", "radialGradient":
		return true
	}
	return false
}

func clearGroupStroke(n *node) {
	if n.Name.Local == "g" {
		if v, ok := n.attr("stroke"); ok && v != "none" {
			n.set("stroke", "currentColor")
		}
		n.del("stroke-width")
	}
	for _, c := range n.Children {
		clearGroupStroke(c)
	}
}

func samePaintStroke(a, b paint) bool {
	return a.stroke == b.stroke && a.width == b.width && a.cap == b.cap && a.join == b.join && a.opacity == b.opacity && a.strokeOp == b.strokeOp
}

func main() {
	if len(os.Args) != 3 {
		log.Fatal("usage: genicons <svg dir> <output dir>")
	}
	src, out := os.Args[1], os.Args[2]
	files, err := filepath.Glob(filepath.Join(src, "*.svg"))
	if err != nil {
		log.Fatal(err)
	}
	compiled := map[string]ui.Icon{}
	glyphs := map[string][]string{} // variable name → part expressions
	names := map[string]bool{}
	for _, f := range files {
		base := strings.TrimSuffix(filepath.Base(f), ".svg")
		data, err := os.ReadFile(f)
		if err != nil {
			log.Fatal(err)
		}
		parts, err := compileParts(base, data)
		if err != nil {
			log.Fatal(err)
		}
		prefix := "light"
		name := base
		if n, ok := strings.CutSuffix(base, "_dark"); ok {
			prefix, name = "dark", n
		}
		names[name] = true
		glyph := prefix + upper(name)
		var exprs []string
		for i, p := range parts {
			key := fmt.Sprintf("%sPart%d", glyph, i)
			compiled[key] = p.icon
			exprs = append(exprs, fmt.Sprintf("{%s, ui.Color{Red: %g, Green: %g, Blue: %g, Alpha: %g}}", upper(key), p.tint.Red, p.tint.Green, p.tint.Blue, p.tint.Alpha))
		}
		glyphs[glyph] = exprs
	}
	code, err := iconcompile.Generate("icons", compiled, "Compiled from IntelliJ Platform icons (Apache License 2.0); see assets/icons/intellij/NOTICE.md.")
	if err != nil {
		log.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(out, "compiled.go"), code, 0o644); err != nil {
		log.Fatal(err)
	}
	var list []string
	for n := range names {
		list = append(list, n)
	}
	sort.Strings(list)
	var b strings.Builder
	b.WriteString("// Code generated by tools/genicons. DO NOT EDIT.\n\npackage icons\n\nimport \"github.com/vivaui/viva/ui\"\n\n")
	var glyphNames []string
	for g := range glyphs {
		glyphNames = append(glyphNames, g)
	}
	sort.Strings(glyphNames)
	b.WriteString("var (\n")
	for _, g := range glyphNames {
		fmt.Fprintf(&b, "\t%s = Glyph{%s}\n", g, strings.Join(glyphs[g], ", "))
	}
	b.WriteString(")\n\n// Icon pairs, one per IntelliJ icon name.\nvar (\n")
	for _, n := range list {
		light, dark := "light"+upper(n), "dark"+upper(n)
		if _, ok := glyphs[dark]; !ok {
			dark = light
		}
		if _, ok := glyphs[light]; !ok {
			light = dark
		}
		fmt.Fprintf(&b, "\t%s = Pair{%s, %s}\n", upper(n), light, dark)
	}
	b.WriteString(")\n")
	if err := os.WriteFile(filepath.Join(out, "pairs.go"), []byte(b.String()), 0o644); err != nil {
		log.Fatal(err)
	}
	fmt.Printf("genicons: %d icons, %d glyphs, %d parts\n", len(list), len(glyphs), len(compiled))
}

func upper(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}
