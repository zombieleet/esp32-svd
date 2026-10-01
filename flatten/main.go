// flatten rewrites SVD files in place so gen-device-svd can generate every
// register. It expands registers that use derivedFrom into full copies, and
// moves clusters nested in an arrayed cluster up into their parent.
//
// Usage: go run flatten/main.go svd/<chip>.svd...
package main

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
)

// node keeps the original bytes of each token, so unchanged parts of the
// file are written back exactly as they were read.
type node struct {
	name     string // element name, empty for text, comments and other tokens
	start    []byte // start tag, or the whole token for non-elements
	end      []byte // end tag
	children []*node
}

func main() {
	exitCode := 0
	for _, path := range os.Args[1:] {
		if err := flattenFile(path); err != nil {
			fmt.Fprintf(os.Stderr, "%s: %v\n", path, err)
			exitCode = 1
		}
	}
	os.Exit(exitCode)
}

func flattenFile(path string) error {
	src, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	root, err := parse(src)
	if err != nil {
		return err
	}
	device := root.child("device")
	if device == nil {
		return fmt.Errorf("no device element")
	}
	peripherals := device.child("peripherals")
	if peripherals == nil {
		return fmt.Errorf("no peripherals element")
	}
	byName := map[string]*node{}
	for _, p := range peripherals.elements("peripheral") {
		byName[p.text("name")] = p
	}
	for _, p := range peripherals.elements("peripheral") {
		regs := p.child("registers")
		if regs == nil {
			continue
		}
		if err := flattenClusters(regs, false); err != nil {
			return fmt.Errorf("%s: %w", p.text("name"), err)
		}
		if err := expandDerived(regs, byName); err != nil {
			return fmt.Errorf("%s: %w", p.text("name"), err)
		}
	}
	var out bytes.Buffer
	root.write(&out)
	return os.WriteFile(path, out.Bytes(), 0o644)
}

// flattenClusters replaces each cluster without dim that sits inside an
// arrayed cluster with its registers, prefixed with the cluster name.
func flattenClusters(block *node, inArray bool) error {
	var children []*node
	for i, c := range block.children {
		if c.name != "cluster" {
			children = append(children, c)
			continue
		}
		dimmed := c.child("dim") != nil
		if !inArray || dimmed {
			if err := flattenClusters(c, dimmed); err != nil {
				return err
			}
			children = append(children, c)
			continue
		}
		base, err := parseNum(c.text("addressOffset"))
		if err != nil {
			return fmt.Errorf("cluster %s: %w", c.text("name"), err)
		}
		indent := whitespaceBefore(block.children, i)
		inner := []byte("\n")
		for j, r := range c.children {
			if r.name == "register" {
				inner = whitespaceBefore(c.children, j)
				break
			}
		}
		first := true
		for _, r := range c.children {
			if r.name == "cluster" {
				return fmt.Errorf("cluster %s: nested cluster %s not supported", c.text("name"), r.text("name"))
			}
			if r.name != "register" {
				continue
			}
			off, err := parseNum(r.text("addressOffset"))
			if err != nil {
				return fmt.Errorf("register %s: %w", r.text("name"), err)
			}
			r = r.clone()
			r.dedent(len(lastLine(inner)) - len(lastLine(indent)))
			r.setText("name", c.text("name")+"_"+r.text("name"))
			r.setText("addressOffset", fmt.Sprintf("0x%X", base+off))
			if !first {
				children = append(children, &node{start: indent})
			}
			children = append(children, r)
			first = false
		}
	}
	block.children = children
	return nil
}

// expandDerived replaces each register that uses derivedFrom with a copy of
// its source, keeping the elements the derived register sets itself.
func expandDerived(block *node, peripherals map[string]*node) error {
	for i, r := range block.children {
		if r.name == "cluster" {
			if err := expandDerived(r, peripherals); err != nil {
				return err
			}
			continue
		}
		if r.name != "register" {
			continue
		}
		from := r.attr("derivedFrom")
		if from == "" {
			continue
		}
		src := findRegister(block, peripherals, from)
		if src == nil {
			return fmt.Errorf("register %s: derivedFrom %s not found", r.text("name"), from)
		}
		if src.attr("derivedFrom") != "" {
			return fmt.Errorf("register %s: derivedFrom chain via %s not supported", r.text("name"), from)
		}
		copied := src.clone()
		for _, c := range r.children {
			if c.name == "" {
				continue
			}
			if !copied.replaceChild(c) {
				copied.appendChild(c)
			}
		}
		block.children[i] = copied
	}
	return nil
}

// findRegister looks up a derivedFrom path. A single name is a sibling in the
// same block, otherwise the first part is a peripheral name.
func findRegister(block *node, peripherals map[string]*node, path string) *node {
	parts := strings.Split(path, ".")
	if len(parts) == 1 {
		return block.named("register", parts[0])
	}
	p := peripherals[parts[0]]
	for p != nil && p.child("registers") == nil && p.attr("derivedFrom") != "" {
		p = peripherals[p.attr("derivedFrom")]
	}
	if p == nil {
		return nil
	}
	n := p.child("registers")
	for i, part := range parts[1:] {
		kind := "cluster"
		if i == len(parts)-2 {
			kind = "register"
		}
		if n = n.named(kind, part); n == nil {
			return nil
		}
	}
	return n
}

func parse(src []byte) (*node, error) {
	d := xml.NewDecoder(bytes.NewReader(src))
	root := &node{}
	stack := []*node{root}
	for {
		startOff := d.InputOffset()
		tok, err := d.RawToken()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		raw := src[startOff:d.InputOffset()]
		top := stack[len(stack)-1]
		switch t := tok.(type) {
		case xml.StartElement:
			n := &node{name: t.Name.Local, start: raw}
			top.children = append(top.children, n)
			stack = append(stack, n)
		case xml.EndElement:
			top.end = raw
			stack = stack[:len(stack)-1]
		default:
			top.children = append(top.children, &node{start: raw})
		}
	}
	return root, nil
}

func (n *node) write(w *bytes.Buffer) {
	w.Write(n.start)
	for _, c := range n.children {
		c.write(w)
	}
	w.Write(n.end)
}

func (n *node) clone() *node {
	c := *n
	c.children = make([]*node, len(n.children))
	for i, ch := range n.children {
		c.children[i] = ch.clone()
	}
	return &c
}

func (n *node) child(name string) *node {
	for _, c := range n.children {
		if c.name == name {
			return c
		}
	}
	return nil
}

func (n *node) elements(name string) []*node {
	var list []*node
	for _, c := range n.children {
		if c.name == name {
			list = append(list, c)
		}
	}
	return list
}

func (n *node) named(kind, name string) *node {
	for _, c := range n.children {
		if c.name == kind && c.text("name") == name {
			return c
		}
	}
	return nil
}

// text returns the text content of a direct child element.
func (n *node) text(name string) string {
	c := n.child(name)
	if c == nil {
		return ""
	}
	var b bytes.Buffer
	for _, t := range c.children {
		b.Write(t.start)
	}
	return strings.TrimSpace(b.String())
}

func (n *node) setText(name, value string) {
	c := n.child(name)
	var b bytes.Buffer
	xml.EscapeText(&b, []byte(value))
	c.children = []*node{{start: b.Bytes()}}
}

func (n *node) attr(name string) string {
	d := xml.NewDecoder(bytes.NewReader(n.start))
	tok, err := d.RawToken()
	if err != nil {
		return ""
	}
	if se, ok := tok.(xml.StartElement); ok {
		for _, a := range se.Attr {
			if a.Name.Local == name {
				return a.Value
			}
		}
	}
	return ""
}

func (n *node) replaceChild(c *node) bool {
	for i, x := range n.children {
		if x.name == c.name {
			n.children[i] = c.clone()
			return true
		}
	}
	return false
}

// appendChild adds c before the trailing whitespace of n, using the same
// indentation as the existing children.
func (n *node) appendChild(c *node) {
	last := len(n.children)
	for last > 0 && n.children[last-1].name == "" && len(bytes.TrimSpace(n.children[last-1].start)) == 0 {
		last--
	}
	indent := whitespaceBefore(n.children, last-1)
	tail := append([]*node{}, n.children[last:]...)
	n.children = append(n.children[:last], &node{start: indent}, c.clone())
	n.children = append(n.children, tail...)
}

// whitespaceBefore returns the whitespace text just before children[i].
func whitespaceBefore(children []*node, i int) []byte {
	if i > 0 && children[i-1].name == "" && len(bytes.TrimSpace(children[i-1].start)) == 0 {
		return children[i-1].start
	}
	return []byte("\n")
}

// dedent removes count spaces after each newline in the whitespace of n's subtree.
func (n *node) dedent(count int) {
	if count <= 0 {
		return
	}
	from := []byte("\n" + strings.Repeat(" ", count))
	for _, c := range n.children {
		if c.name == "" && len(bytes.TrimSpace(c.start)) == 0 {
			c.start = bytes.ReplaceAll(c.start, from, []byte("\n"))
		}
		c.dedent(count)
	}
}

func lastLine(b []byte) []byte {
	return b[bytes.LastIndexByte(b, '\n')+1:]
}

func parseNum(s string) (uint64, error) {
	return strconv.ParseUint(strings.ToLower(s), 0, 64)
}
