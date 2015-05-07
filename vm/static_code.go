package vm

var staticCode = `
//+pigeon: err.go

var (
	// errInvalidEncoding is returned when the source is not properly
	// utf8-encoded.
	errInvalidEncoding = errors.New("invalid encoding")

	// errNoMatch is returned if no match could be found and no other
	// error has been raised.
	errNoMatch = errors.New("no match found")
)

// errList cumulates the errors found by the parser. It is part
// of the supported API.
type errList []error

// ϡadd adds err to the list of errors.
func (e *errList) ϡadd(err error) {
	if err != nil {
		*e = append(*e, err)
	}
}

// ϡerr returns the error list as an error, or nil if the list is empty.
func (e errList) ϡerr() error {
	if len(e) == 0 {
		return nil
	}
	e.ϡdedupe()
	return e
}

// ϡdedupe removes duplicate error messages from the list.
func (e *errList) ϡdedupe() {
	var cleaned []error
	set := make(map[string]bool)
	for _, err := range *e {
		if msg := err.Error(); !set[msg] {
			set[msg] = true
			cleaned = append(cleaned, err)
		}
	}
	*e = cleaned
}

// Error returns the error message for the errList. It implements the
// error interface.
func (e errList) Error() string {
	var buf bytes.Buffer

	for i, err := range e {
		if i > 0 {
			buf.WriteRune('\n')
		}
		buf.WriteString(err.Error())
	}
	return buf.String()
}

// parserError wraps an error with a prefix indicating the rule in which
// the error occurred. The original error is stored in the Inner field.
// It is part of the supported API.
type parserError struct {
	Inner   error
	ϡpos    position
	ϡprefix string
}

// Error returns the prefixed error message. It implements the error
// interface.
func (p *parserError) Error() string {
	return p.ϡprefix + ": " + p.Inner.Error()
}
//+pigeon: matchers.go

// ϡpeekReader is the interface that defines the peek and read
// methods.
type ϡpeekReader interface {
	peek() rune
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
	rn := pr.peek()
	pr.read()
	return rn != utf8.RuneError
}

func (a ϡanyMatcher) String() string {
	return "."
}

// ϡstringMatcher is a matcher that matches a string.
type ϡstringMatcher struct {
	ignoreCase bool
	value      string // value must be lowercase if ignoreCase is true
}

// match tries to match the string in the peekReader.
func (s ϡstringMatcher) match(pr ϡpeekReader) bool {
	for _, want := range s.value {
		rn := pr.peek()
		if s.ignoreCase {
			rn = unicode.ToLower(rn)
		}
		if rn != want {
			return false
		}
		pr.read()
	}
	return true
}

func (s ϡstringMatcher) String() string {
	v := strconv.Quote(s.value)
	if s.ignoreCase {
		v += "i"
	}
	return v
}

// ϡcharClassMatcher is a matcher that matches classes of characters.
type ϡcharClassMatcher struct {
	chars   []rune // runes must be lowercase if ignoreCase is true
	ranges  []rune // same for ranges
	classes []*unicode.RangeTable

	ignoreCase bool
	inverted   bool
}

func (c ϡcharClassMatcher) String() string {
	var buf bytes.Buffer

	buf.WriteString("[")
	if c.inverted {
		buf.WriteString("^")
	}
	for _, c := range c.chars {
		buf.WriteRune(c)
	}
	for i := 0; i < len(c.ranges); i += 2 {
		buf.WriteString(fmt.Sprintf("%c-%c", c.ranges[i], c.ranges[i+1]))
	}
	// unicode classes can't be stringified
	if l := len(c.classes); l > 0 {
		buf.WriteString(fmt.Sprintf("\\p{%d classes}", l))
	}
	buf.WriteString("]")
	if c.ignoreCase {
		buf.WriteString("i")
	}
	return buf.String()
}

// match tries to match classes of characters in the peekReader.
func (c ϡcharClassMatcher) match(pr ϡpeekReader) bool {
	rn := pr.peek()
	pr.read()

	if c.ignoreCase {
		rn = unicode.ToLower(rn)
	}

	// try to match in the list of available chars
	for _, ch := range c.chars {
		if rn == ch {
			return !c.inverted
		}
	}

	// try to match in the list of ranges
	for i := 0; i < len(c.ranges); i += 2 {
		if rn >= c.ranges[i] && rn <= c.ranges[i+1] {
			return !c.inverted
		}
	}

	// try to match in the list of Unicode classes
	for _, cl := range c.classes {
		if unicode.Is(cl, rn) {
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
//+pigeon: ops.go

// ϡop represents an opcode.
type ϡop byte

// list of opcodes in the pigeon VM.
const (
	ϡopExit ϡop = iota
	ϡopCall
	ϡopCallA
	ϡopCallB
	ϡopCumulOrF
	ϡopJump
	ϡopJumpIfF
	ϡopJumpIfT
	ϡopMatch
	ϡopNilIfF
	ϡopNilIfT
	ϡopPop
	ϡopPopVJumpIfF
	ϡopPush
	ϡopRestore
	ϡopRestoreIfF
	ϡopReturn
	ϡopStoreIfT
	ϡopTakeLOrJump
	ϡopmax // must always be after the last valid opcode

	// ϡopPlaceholder is an (invalid) opcode used by the Generator
	// to insert opcodes that need the index of the starting instruction
	// of a rule that hasn't been generated yet.
	//
	// It must be placed after ϡopmax (because it is invalid in the
	// final program) and it has one argument, the index in the strings
	// array of the identifier of the rule.
	ϡopPlaceholder
)

// ϡlookupOp translates an opcode to a string.
var ϡlookupOp = []string{
	ϡopExit: "exit", ϡopCall: "call", ϡopCallA: "callA",
	ϡopCallB: "callB", ϡopCumulOrF: "cumulOrF",
	ϡopJump: "jump", ϡopJumpIfF: "jumpIfF", ϡopJumpIfT: "jumpIfT",
	ϡopMatch: "match", ϡopNilIfF: "nilIfF", ϡopNilIfT: "nilIfT",
	ϡopPop: "pop", ϡopPopVJumpIfF: "popVJumpIfF",
	ϡopPush: "push", ϡopRestore: "restore", ϡopRestoreIfF: "restoreIfF",
	ϡopReturn: "return", ϡopStoreIfT: "storeIfT", ϡopTakeLOrJump: "takeLOrJump",
}

// String returns the string representation of the opcode.
func (op ϡop) String() string {
	if 0 <= op && int(op) < len(ϡlookupOp) {
		return ϡlookupOp[op]
	}
	return "ϡop(" + strconv.Itoa(int(op)) + ")"
}

// ϡinstr holds a single instruction: an opcode with its arguments.
type ϡinstr struct {
	op       ϡop
	ruleNmIx int // because bootstrap instructions have rule index -1
	args     []uint16
}

// String returns the string representation of the instruction.
func (ins ϡinstr) String() string {
	var buf bytes.Buffer

	buf.WriteString("{" + strconv.Itoa(int(ins.op)) + ", " +
		strconv.Itoa(int(ins.ruleNmIx)) + ", ")

	if len(ins.args) > 0 {
		buf.WriteString("[]uint16{")
		for i, arg := range ins.args {
			if i > 0 {
				buf.WriteString(", ")
			}
			buf.WriteString(strconv.Itoa(int(arg)))
		}
		buf.WriteString("}")
	} else {
		buf.WriteString("nil")
	}

	buf.WriteString("}")
	return buf.String()
}
//+pigeon: parser.go

// position records a position in the text. It is part of the supported
// API.
type position struct {
	// line is the 1-based index of the line of the current rune.
	line int
	// col is the 1-based index of the current rune on the line.
	col int
	// offset is the 0-based index of the starting byte of the current rune.
	offset int
}

// String formats a position as a string.
func (p position) String() string {
	return fmt.Sprintf("%d:%d (%d)", p.line, p.col, p.offset)
}

// current represents current matching data. It is the value on which
// action and predicate code blocks are generated as methods. It is
// part of the supported API.
type current struct {
	// pos holds the start position of the current match.
	pos position
	// text contains the raw text of the match. It is a slice in the
	// source data, so it should not be modified.
	text []byte
}

// ϡsvpt stores all state required to go back to a point in the
// parser.
type ϡsvpt struct {
	position
	rn rune
	w  int
}

// ϡparser parses the input text as rune code points.
type ϡparser struct {
	data []byte
	pt   ϡsvpt
	cur  current
}

// peek returns the current rune.
func (p *ϡparser) peek() rune {
	return p.pt.rn
}

// read advances the parser to the next rune.
func (p *ϡparser) read() {
	p.pt.offset += p.pt.w
	rn, n := utf8.DecodeRune(p.data[p.pt.offset:])
	p.pt.rn = rn
	p.pt.w = n

	if rn == utf8.RuneError {
		if n > 0 {
			panic(errInvalidEncoding)
		}
	} else {
		p.pt.col++
		if rn == '\n' {
			p.pt.line++
			p.pt.col = 0
		}
	}
}

// sliceFrom gets the slice of bytes from the start savepoint to
// the current position, non inclusive.
func (p *ϡparser) sliceFrom(start ϡsvpt) []byte {
	return p.data[start.position.offset:p.pt.position.offset]
}
//+pigeon: pub.go

// Option is a function that can set an option on the parser. It returns
// the previous setting as an Option.
type Option func(*ϡvm) Option

// Debug creates an Option to set the debug flag to b. When set to true,
// debugging information is printed to stdout while parsing.
//
// The default is false.
func Debug(b bool) Option {
	return func(v *ϡvm) Option {
		old := v.debug
		v.debug = b
		return Debug(old)
	}
}

// Memoize creates an Option to set the memoize flag to b. When set to true,
// the parser will cache all results so each expression is evaluated only
// once. This guarantees linear parsing time even for pathological cases,
// at the expense of more memory and slower times for typical cases.
//
// The default is false.
func Memoize(b bool) Option {
	return func(v *ϡvm) Option {
		old := v.memoize
		v.memoize = b
		return Memoize(old)
	}
}

// Recover creates an Option to set the recover flag to b. When set to
// true, this causes the parser to recover from panics and convert it
// to an error. Setting it to false can be useful while debugging to
// access the full stack trace.
//
// The default is true.
func Recover(b bool) Option {
	return func(v *ϡvm) Option {
		old := v.recover
		v.recover = b
		return Recover(old)
	}
}

// ParseFile parses the file identified by filename.
func ParseFile(filename string, opts ...Option) (interface{}, error) {
	f, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return ParseReader(filename, f, opts...)
}

// ParseReader parses the data from r using filename as information in the
// error messages.
func ParseReader(filename string, r io.Reader, opts ...Option) (interface{}, error) {
	b, err := ioutil.ReadAll(r)
	if err != nil {
		return nil, err
	}

	return Parse(filename, b, opts...)
}

// Parse parses the data from b using filename as information in the
// error messages.
func Parse(filename string, b []byte, opts ...Option) (interface{}, error) {
	p := &ϡparser{
		data: b,
		pt:   ϡsvpt{position: position{line: 1}},
	}
	v := &ϡvm{
		filename: filename,
		parser:   p,
		recover:  true,
	}
	return v.setOptions(opts).run(ϡtheProgram)
}
//+pigeon: stacks.go

// ϡpstack implements the Position stack. It stores savepoints.
type ϡpstack struct {
	ar []ϡsvpt
	sp int
}

// push adds a value on the stack.
func (p *ϡpstack) push(pt ϡsvpt) {
	if p.sp >= len(p.ar) {
		p.ar = append(p.ar, pt)
	} else {
		p.ar[p.sp] = pt
	}
	p.sp++
}

// pop removes a value from the stack.
func (p *ϡpstack) pop() ϡsvpt {
	p.sp--
	return p.ar[p.sp]
}

func (p *ϡpstack) len() int {
	return p.sp
}

func newPstack(cap int) *ϡpstack {
	return &ϡpstack{ar: make([]ϡsvpt, cap)}
}

// ϡistack implements the Instruction index stack. It stores integers.
type ϡistack struct {
	ar []uint16
	sp int
}

// push adds a value on the stack.
func (i *ϡistack) push(v uint16) {
	if i.sp >= len(i.ar) {
		i.ar = append(i.ar, v)
	} else {
		i.ar[i.sp] = v
	}
	i.sp++
}

// pop removes a value from the stack.
func (i *ϡistack) pop() uint16 {
	i.sp--
	return i.ar[i.sp]
}

func (i *ϡistack) len() int {
	return i.sp
}

func newIstack(cap int) *ϡistack {
	return &ϡistack{ar: make([]uint16, cap)}
}

// ϡvstack implements the Value stack. It stores empty interfaces.
type ϡvstack struct {
	ar []interface{}
	sp int
}

// push adds a value on the stack.
func (v *ϡvstack) push(i interface{}) {
	if v.sp >= len(v.ar) {
		v.ar = append(v.ar, i)
	} else {
		v.ar[v.sp] = i
	}
	v.sp++
}

// pop removes a value from the stack.
func (v *ϡvstack) pop() interface{} {
	v.sp--
	return v.ar[v.sp]
}

// peek returns the value at the top of the stack, leaving it there.
func (v *ϡvstack) peek() interface{} {
	return v.ar[v.sp-1]
}

func (v *ϡvstack) len() int {
	return v.sp
}

func newVstack(cap int) *ϡvstack {
	return &ϡvstack{ar: make([]interface{}, cap)}
}

// ϡlstack implements the Loop stack. It stores slices of integers.
type ϡlstack struct {
	ar [][]uint16
	sp int
}

// push adds a value on the stack.
func (l *ϡlstack) push(a []uint16) {
	if l.sp >= len(l.ar) {
		l.ar = append(l.ar, a)
	} else {
		l.ar[l.sp] = a
	}
	l.sp++
}

// pop removes a value from the stack.
func (l *ϡlstack) pop() []uint16 {
	l.sp--
	return l.ar[l.sp]
}

// take removes the integer at index 0 from the slice at the top of the
// stack. It returns -1 if the slice is empty. The slice is left on the
// stack.
func (l *ϡlstack) take() int {
	v := -1
	a := l.ar[l.sp-1]
	if len(a) > 0 {
		v = int(a[0])
		l.ar[l.sp-1] = a[1:]
	}
	return v
}

func (l *ϡlstack) len() int {
	return l.sp
}

func newLstack(cap int) *ϡlstack {
	return &ϡlstack{ar: make([][]uint16, cap)}
}

// ϡargsSet holds the list of arguments (key and value) to pass
// to the code blocks.
type ϡargsSet map[string]interface{}

// ϡastack is a stack of ϡargsSet.
type ϡastack struct {
	ar []ϡargsSet
	sp int
}

// push adds an empty ϡargsSet on top of the stack.
func (a *ϡastack) push() {
	if a.sp >= len(a.ar) {
		a.ar = append(a.ar, nil)
	} else {
		a.ar[a.sp] = nil
	}
	a.sp++
}

// pop removes the top ϡargsSet from the stack.
func (a *ϡastack) pop() {
	a.sp--
}

// peek returns the current top ϡargsSet.
func (a *ϡastack) peek() ϡargsSet {
	as := a.ar[a.sp-1]
	if as == nil {
		as = make(ϡargsSet)
		a.ar[a.sp-1] = as
	}
	return as
}

func (a *ϡastack) len() int {
	return a.sp
}

func newAstack(cap int) *ϡastack {
	return &ϡastack{ar: make([]ϡargsSet, cap)}
}
//+pigeon: vm.go

// ϡsentinel is a type used to define sentinel values that shouldn't
// be equal to something else.
type ϡsentinel int

const (
	// ϡmatchFailed is a sentinel value used to indicate a match failure.
	ϡmatchFailed ϡsentinel = iota - 1
)

const (
	// stack IDs, used in PUSH and POP's first argument
	ϡpstackID = iota + 1
	ϡlstackID
	ϡvstackID
	ϡistackID
	ϡastackID

	// special V stack values
	ϡvValNil    uint16 = 0
	ϡvValFailed uint16 = 1
	ϡvValEmpty  uint16 = 2
)

var (
	ϡstackNm = []string{
		ϡpstackID: "P",
		ϡlstackID: "L",
		ϡvstackID: "V",
		ϡistackID: "I",
		ϡastackID: "A",
	}
)

// special values that may be pushed on the V stack.
var ϡvSpecialValues = []interface{}{
	nil,
	ϡmatchFailed,
	[]interface{}(nil),
}

// ϡmemoizedResult holds the state required to reuse a memoized result.
type ϡmemoizedResult struct {
	v  interface{}
	pt ϡsvpt
}

// ϡffp holds state to record the farthest failure position.
type ϡffp struct {
	pos      position
	ruleNmIx int
	rn       rune
	want     string
}

// err returns an error if a farthest failure position has been recorded,
// nil otherwise.
func (ffp ϡffp) err() error {
	if ffp.pos.offset >= 0 {
		return ffp
	}
	return nil
}

// Error implements the error interface.
func (ffp ϡffp) Error() string {
	return fmt.Sprintf("expected %q, got %#U", ffp.want, ffp.rn)
}

// ϡprogram is the data structure that is generated by the builder
// based on an input PEG. It contains the program information required
// to execute the grammar using the vm.
type ϡprogram struct {
	instrs []ϡinstr

	// lists
	ms []ϡmatcher
	as []func(*ϡvm) (interface{}, error)
	bs []func(*ϡvm) (bool, error)
	ss []string
}

// String formats the program's instructions in a human-readable format.
func (pg ϡprogram) String() string {
	var buf bytes.Buffer

	for i, instr := range pg.instrs {
		buf.WriteString(fmt.Sprintf("[%3d]: %s\n", i, pg.instrToString(instr)))
	}
	return buf.String()
}

// instrToString formats an instruction in a human-readable format, in the
// context of the program.
func (pg ϡprogram) instrToString(instr ϡinstr) string {
	var buf bytes.Buffer

	rule := pg.ruleNameAt(instr.ruleNmIx)
	if rule == "" {
		rule = "<bootstrap>"
	}
	buf.WriteString(fmt.Sprintf("%s.%s %v", rule, instr.op, instr.args))
	switch instr.op {
	case ϡopPush, ϡopPop:
		buf.WriteString(" " + ϡstackNm[instr.args[0]])
	case ϡopMatch:
		buf.WriteString(fmt.Sprintf(" %s", pg.ms[instr.args[0]]))
	case ϡopStoreIfT:
		buf.WriteString(" " + pg.ss[instr.args[0]])
	}
	return buf.String()
}

// ruleNameAt returns the name of the rule that contains the instruction
// index. It returns an empty string is the instruction is not part of a
// rule (bootstrap instruction, invalid index).
func (pg ϡprogram) ruleNameAt(ix int) string {
	if ix < 0 || ix >= len(pg.ss) {
		return ""
	}
	return pg.ss[ix]
}

// ϡvm holds the state to execute a compiled grammar.
type ϡvm struct {
	// input
	filename string
	parser   *ϡparser

	// options
	debug   bool
	memoize bool
	recover bool

	// program data
	pc  uint16
	pg  *ϡprogram
	cur current

	// stacks
	p *ϡpstack
	l *ϡlstack
	v *ϡvstack
	i *ϡistack
	a *ϡastack

	// memoized results: by instruction index, then by byte offset
	memo map[uint16]map[int]ϡmemoizedResult
	ffp  ϡffp
	// TODO: farthest failure position

	// stats
	matchCnt    int
	callCnt     int
	actionCnt   int
	codePredCnt int

	// error list
	errs errList
}

func (v *ϡvm) fromMemo(ix uint16, pt ϡsvpt) bool {
	if v.memo == nil {
		return false
	}
	m := v.memo[ix]
	if m == nil {
		return false
	}
	result, ok := m[pt.offset]
	if ok {
		v.parser.pt = result.pt
		v.v.push(result.v)
		return true
	}
	return false
}

func (v *ϡvm) memoizeMatch(ix uint16, pt ϡsvpt, match bool) {
	if v.memo == nil {
		v.memo = make(map[uint16]map[int]ϡmemoizedResult)
	}
	m, ok := v.memo[ix]
	if !ok {
		m = make(map[int]ϡmemoizedResult)
		v.memo[ix] = m
	}

	if match {
		m[pt.offset] = ϡmemoizedResult{v.parser.sliceFrom(pt), v.parser.pt}
		return
	}
	m[pt.offset] = ϡmemoizedResult{ϡmatchFailed, pt}
}

// setOptions applies the options in sequence on the vm. It returns the
// vm to allow for chaining calls.
func (v *ϡvm) setOptions(opts []Option) *ϡvm {
	for _, opt := range opts {
		opt(v)
	}
	return v
}

// addErr adds the error at the current parser position, without rule name
// information.
func (v *ϡvm) addErr(err error) {
	v.addErrAt(err, -1, v.parser.pt.position)
}

// addErrAt adds the error at the specified position, for the rule name at
// ruleNmIx.
func (v *ϡvm) addErrAt(err error, ruleNmIx int, pos position) {
	var buf bytes.Buffer
	if v.filename != "" {
		buf.WriteString(v.filename)
	}
	if buf.Len() > 0 {
		buf.WriteString(":")
	}
	buf.WriteString(fmt.Sprintf("%s", pos))

	ruleNm := v.pg.ruleNameAt(ruleNmIx)
	if ruleNm != "" {
		buf.WriteString(": ")
		buf.WriteString("rule " + ruleNm)
	}

	pe := &parserError{Inner: err, ϡprefix: buf.String()}
	v.errs.ϡadd(pe)
}

// dumpSnapshot writes a dump of the current VM state to w.
func (v *ϡvm) dumpSnapshot(w io.Writer) {
	var buf bytes.Buffer

	if v.filename != "" {
		buf.WriteString(v.filename + ":")
	}
	buf.WriteString(fmt.Sprintf("%s: %#U\n", v.parser.pt.position, v.parser.pt.rn))

	// write the next 5 instructions
	ix := v.pc - 1
	if ix > 0 {
		ix--
	}
	stdFmt := ". [%d]: %s"
	for i := 0; i < 5; i++ {
		stdFmt := stdFmt
		if ix == v.pc-1 {
			stdFmt = ">" + stdFmt[1:]
		}
		instr := v.pg.instrs[ix]
		switch instr.op {
		case ϡopCall:
			buf.WriteString(fmt.Sprintf(stdFmt+"\n", ix, v.pg.instrToString(instr)))
			ix = v.i.pop() // continue with instructions at this index
			v.i.push(ix)
			continue
		default:
			buf.WriteString(fmt.Sprintf(stdFmt+"\n", ix, v.pg.instrToString(instr)))
		}
		ix++
		if int(ix) >= len(v.pg.instrs) {
			break
		}
	}

	// // print the stacks
	// buf.WriteString("[ P: ")
	// for i := 0; i < 3; i++ {
	// 	if len(v.p) <= i {
	// 		break
	// 	}
	// 	if i > 0 {
	// 		buf.WriteString(", ")
	// 	}
	// 	val := v.p[len(v.p)-i-1]
	// 	buf.WriteString(fmt.Sprintf("\"%v\"", val))
	// }
	// buf.WriteString(" ]\n[ V: ")
	// for i := 0; i < 3; i++ {
	// 	if len(v.v) <= i {
	// 		break
	// 	}
	// 	if i > 0 {
	// 		buf.WriteString(", ")
	// 	}
	// 	val := v.v[len(v.v)-i-1]
	// 	buf.WriteString(fmt.Sprintf("%#v", val))
	// }
	// buf.WriteString(" ]\n[ I: ")
	// for i := 0; i < 3; i++ {
	// 	if len(v.i) <= i {
	// 		break
	// 	}
	// 	if i > 0 {
	// 		buf.WriteString(", ")
	// 	}
	// 	val := v.i[len(v.i)-i-1]
	// 	buf.WriteString(fmt.Sprintf("%d", val))
	// }
	// buf.WriteString(" ]\n[ L: ")
	// for i := 0; i < 3; i++ {
	// 	if len(v.l) <= i {
	// 		break
	// 	}
	// 	if i > 0 {
	// 		buf.WriteString(", ")
	// 	}
	// 	val := v.l[len(v.l)-i-1]
	// 	buf.WriteString(fmt.Sprintf("%v", val))
	// }
	// buf.WriteString(" ]\n")
	fmt.Fprintln(w, buf.String())
}

// run executes the provided program in this VM, and returns the result.
func (v *ϡvm) run(pg *ϡprogram) (interface{}, error) {
	v.pg = pg
	v.a = newAstack(128)
	v.i = newIstack(128)
	v.v = newVstack(128)
	v.l = newLstack(128)
	v.p = newPstack(128)
	v.ffp.pos.offset = -1
	ret := v.dispatch()

	// if the match failed, translate that to a nil result and make
	// sure it returns an error
	if ret == ϡmatchFailed {
		ret = nil
		if len(v.errs) == 0 {
			if err := v.ffp.err(); err != nil {
				v.addErrAt(err, v.ffp.ruleNmIx, v.ffp.pos)
			} else {
				v.addErr(errNoMatch)
			}
		}
	}

	return ret, v.errs.ϡerr()
}

// dispatch is the proper execution method of the VM, it loops over
// the instructions and executes each opcode.
func (v *ϡvm) dispatch() interface{} {
	var instrPath []uint16
	if v.debug {
		fmt.Fprintln(os.Stderr, v.pg)
		defer func() {
			var buf bytes.Buffer

			buf.WriteString("Execution path:\n")
			for _, ix := range instrPath {
				buf.WriteString(fmt.Sprintf("[%3d]: %s\n", ix, v.pg.instrToString(v.pg.instrs[ix])))
			}
			fmt.Fprintln(os.Stderr, buf.String())
		}()
	}

	if v.recover {
		// if recover is set, recover from panics and convert to error.
		defer func() {
			if e := recover(); e != nil {
				ruleIx := -1
				if v.pc > 0 {
					ruleIx = v.pg.instrs[v.pc-1].ruleNmIx
				}
				switch e := e.(type) {
				case error:
					v.addErrAt(e, ruleIx, v.parser.pt.position)
				default:
					v.addErrAt(fmt.Errorf("%v", e), ruleIx, v.parser.pt.position)
				}
			}
		}()
	}

	// move to first rune before starting the loop
	v.parser.read()
	for {
		// fetch and decode the instruction
		instr := v.pg.instrs[v.pc]
		instrPath = append(instrPath, v.pc)

		// increment program counter
		v.pc++

		switch instr.op {
		case ϡopCall:
			if v.debug {
				v.dumpSnapshot(os.Stderr)
			}
			ix := v.i.pop()
			v.i.push(v.pc)
			v.pc = ix
			v.callCnt++

		case ϡopCallA:
			if v.debug {
				v.dumpSnapshot(os.Stderr)
			}
			v.v.pop()
			start := v.p.pop()
			v.cur.pos = start.position
			v.cur.text = v.parser.sliceFrom(start)
			if int(instr.args[0]) >= len(v.pg.as) {
				panic(fmt.Sprintf("invalid %s argument: %d", instr.op, instr.args[0]))
			}
			fn := v.pg.as[instr.args[0]]
			val, err := fn(v)
			if err != nil {
				v.addErrAt(err, int(instr.ruleNmIx), start.position)
			}
			v.v.push(val)
			v.actionCnt++

		case ϡopCallB:
			if v.debug {
				v.dumpSnapshot(os.Stderr)
			}
			v.cur.pos = v.parser.pt.position
			v.cur.text = nil
			if int(instr.args[0]) >= len(v.pg.bs) {
				panic(fmt.Sprintf("invalid %s argument: %d", instr.op, instr.args[0]))
			}
			fn := v.pg.bs[instr.args[0]]
			val, err := fn(v)
			if err != nil {
				v.addErrAt(err, int(instr.ruleNmIx), v.parser.pt.position)
			}
			v.codePredCnt++
			if !val {
				v.v.push(ϡmatchFailed)
				break
			}
			v.v.push(nil)

		case ϡopCumulOrF:
			va, vb := v.v.pop(), v.v.pop()
			if va == ϡmatchFailed {
				v.v.push(ϡmatchFailed)
				break
			}
			switch vb := vb.(type) {
			case []interface{}:
				vb = append(vb, va)
				v.v.push(vb)
			case ϡsentinel:
				v.v.push([]interface{}{va})
			default:
				panic(fmt.Sprintf("invalid %s value type on the V stack: %T", instr.op, vb))
			}

		case ϡopExit:
			return v.v.pop()

		case ϡopJump:
			v.pc = instr.args[0]

		case ϡopJumpIfF:
			if top := v.v.peek(); top == ϡmatchFailed {
				v.pc = instr.args[0]
			}

		case ϡopJumpIfT:
			if top := v.v.peek(); top != ϡmatchFailed {
				v.pc = instr.args[0]
			}

		case ϡopMatch:
			start := v.parser.pt
			if v.memoize && v.fromMemo(v.pc-1, start) {
				break
			}

			if int(instr.args[0]) >= len(v.pg.ms) {
				panic(fmt.Sprintf("invalid %s argument: %d", instr.op, instr.args[0]))
			}
			m := v.pg.ms[instr.args[0]]
			ok := m.match(v.parser)
			if v.memoize {
				v.memoizeMatch(v.pc-1, start, ok)
			}
			v.matchCnt++
			if ok {
				v.v.push(v.parser.sliceFrom(start))
				break
			}
			// did not match, record ffp if this is the farthest failure
			if start.offset > v.ffp.pos.offset {
				v.ffp.pos = start.position
				v.ffp.rn = start.rn
				v.ffp.ruleNmIx = instr.ruleNmIx
				v.ffp.want = fmt.Sprintf("%s", m)
			}
			v.v.push(ϡmatchFailed)
			v.parser.pt = start

			if v.debug {
				v.dumpSnapshot(os.Stderr)
			}

		case ϡopNilIfF:
			if top := v.v.pop(); top == ϡmatchFailed {
				v.v.push(nil)
				break
			}
			v.v.push(ϡmatchFailed)

		case ϡopNilIfT:
			if top := v.v.pop(); top != ϡmatchFailed {
				v.v.push(nil)
				break
			}
			v.v.push(ϡmatchFailed)

		case ϡopPop:
			switch instr.args[0] {
			case ϡlstackID:
				v.l.pop()
			case ϡpstackID:
				v.p.pop()
			case ϡastackID:
				v.a.pop()
			case ϡvstackID:
				v.v.pop()
			default:
				panic(fmt.Sprintf("invalid %s argument: %d", instr.op, instr.args[0]))
			}

		case ϡopPopVJumpIfF:
			if top := v.v.peek(); top == ϡmatchFailed {
				v.v.pop()
				v.pc = instr.args[0]
			}

		case ϡopPush:
			switch instr.args[0] {
			case ϡpstackID:
				v.p.push(v.parser.pt)
			case ϡistackID:
				v.i.push(instr.args[1])
			case ϡvstackID:
				if int(instr.args[1]) >= len(ϡvSpecialValues) {
					panic(fmt.Sprintf("invalid %s V stack argument: %d", instr.op, instr.args[1]))
				}
				v.v.push(ϡvSpecialValues[instr.args[1]])
			case ϡastackID:
				v.a.push()
			case ϡlstackID:
				v.l.push(instr.args[1:])
			default:
				panic(fmt.Sprintf("invalid %s argument: %d", instr.op, instr.args[0]))
			}

		case ϡopRestore:
			pt := v.p.pop()
			v.parser.pt = pt

		case ϡopRestoreIfF:
			pt := v.p.pop()
			if top := v.v.peek(); top == ϡmatchFailed {
				v.parser.pt = pt
			}

		case ϡopReturn:
			ix := v.i.pop()
			v.pc = ix

		case ϡopStoreIfT:
			if top := v.v.peek(); top != ϡmatchFailed {
				// get the label name
				if int(instr.args[0]) >= len(v.pg.ss) {
					panic(fmt.Sprintf("invalid %s argument: %d", instr.op, instr.args[0]))
				}
				lbl := v.pg.ss[instr.args[0]]

				// store the value
				as := v.a.peek()
				as[lbl] = top
			}

		case ϡopTakeLOrJump:
			ix := v.l.take()
			if ix < 0 {
				v.pc = instr.args[0]
				break
			}
			v.i.push(uint16(ix))

		default:
			panic(fmt.Sprintf("unknown opcode %s", instr.op))
		}
	}
}
`
