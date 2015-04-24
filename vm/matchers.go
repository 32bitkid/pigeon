package vm

import (
	"fmt"
	"unicode"
	"unicode/utf8"
)

// ϡpeekReader is the interface that defines the peek and read
// methods.
type ϡpeekReader interface {
	peek() ϡsvpt
	read()
}

// ϡmatcher is the interface that defines the match method.
type ϡmatcher interface {
	match(ϡpeekReader) bool
}

// ϡanyMatcher is a matcher that matches any character but the
// EOF.
type ϡanyMatcher struct{}

// match tries to match a character in the peekReader.
func (a ϡanyMatcher) match(pr ϡpeekReader) bool {
	pt := pr.peek()
	pr.read()
	return pt.rn != utf8.RuneError
}

// ϡstringMatcher is a matcher that matches a string.
type ϡstringMatcher struct {
	ignoreCase bool
	value      string // value must be lowercase if ignoreCase is true
}

// match tries to match the string in the peekReader.
func (s ϡstringMatcher) match(pr ϡpeekReader) bool {
	for _, want := range s.value {
		pt := pr.peek()
		if s.ignoreCase {
			pt.rn = unicode.ToLower(pt.rn)
		}
		if pt.rn != want {
			return false
		}
		pr.read()
	}
	return true
}

// ϡcharClassMatcher is a matcher that matches classes of characters.
type ϡcharClassMatcher struct {
	chars   []rune // runes must be lowercase if ignoreCase is true
	ranges  []rune // TODO : document potential issues if ignore case is used with ranges
	classes []*unicode.RangeTable

	ignoreCase bool
	inverted   bool
}

// match tries to match classes of characters in the peekReader.
func (c ϡcharClassMatcher) match(pr ϡpeekReader) bool {
	pt := pr.peek()
	pr.read()

	if c.ignoreCase {
		pt.rn = unicode.ToLower(pt.rn)
	}

	// try to match in the list of available chars
	for _, rn := range c.chars {
		if pt.rn == rn {
			return !c.inverted
		}
	}

	// try to match in the list of ranges
	for i := 0; i < len(c.ranges); i += 2 {
		if pt.rn >= c.ranges[i] && pt.rn <= c.ranges[i+1] {
			return !c.inverted
		}
	}

	// try to match in the list of Unicode classes
	for _, cl := range c.classes {
		if unicode.Is(cl, pt.rn) {
			return !c.inverted
		}
	}

	return c.inverted
}

// ϡrangeTable returns the corresponding unicode range table from the
// provided class name.
func ϡrangeTable(class string) *unicode.RangeTable {
	if rt, ok := unicode.Categories[class]; ok {
		return rt
	}
	if rt, ok := unicode.Properties[class]; ok {
		return rt
	}
	if rt, ok := unicode.Scripts[class]; ok {
		return rt
	}

	// cannot happen
	panic(fmt.Sprintf("invalid Unicode class: %s", class))
}
