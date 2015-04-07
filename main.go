package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/PuerkitoBio/pigeon/ast"
	"github.com/PuerkitoBio/pigeon/builder"
)

func main() {
	// define command-line flags
	var (
		dbgFlag       = flag.Bool("debug", false, "set debug mode")
		shortHelpFlag = flag.Bool("h", false, "show help page")
		longHelpFlag  = flag.Bool("help", false, "show help page")
		outputFlag    = flag.String("o", "", "output file, defaults to stdout")
		recvrNmFlag   = flag.String("receiver-name", "c", "receiver name for the generated methods")
		noBuildFlag   = flag.Bool("x", false, "do not build, only parse")
	)
	flag.Usage = usage
	flag.Parse()

	if *shortHelpFlag || *longHelpFlag {
		flag.Usage()
		os.Exit(0)
	}

	if flag.NArg() > 1 {
		argError(1, "expected one argument, got %q", strings.Join(flag.Args(), " "))
	}

	// get input source
	infile := ""
	if flag.NArg() == 1 {
		infile = flag.Arg(0)
	}
	nm, rc := input(infile)
	defer rc.Close()

	// parse input
	debug = *dbgFlag
	g, err := ParseReader(nm, rc)
	if err != nil {
		fmt.Fprintln(os.Stderr, "parse error(s):\n", err)
		os.Exit(3)
	}

	if !*noBuildFlag {
		// generate parser
		out := output(*outputFlag)
		defer out.Close()

		curNmOpt := builder.ReceiverName(*recvrNmFlag)
		if err := builder.BuildParser(out, g.(*ast.Grammar), curNmOpt); err != nil {
			fmt.Fprintln(os.Stderr, "build error: ", err)
			os.Exit(5)
		}
	}
}

var usagePage = `usage: %s [options] [GRAMMAR_FILE]

Pigeon generates a parser based on a PEG grammar. It doesn't try
to format the generated code nor to detect required imports -
it is recommended to pipe the output of pigeon through a tool
such as goimports to do this, e.g.:

	pigeon GRAMMAR_FILE | goimports > output.go

Use the following command to install goimports:

	go get golang.org/x/tools/cmd/goimports

By default, pigeon reads the grammar from stdin and writes the
generated parser to stdout. If GRAMMAR_FILE is specified, the
grammar is read from this file instead. If the -o flag is set,
the generated code is written to this file instead.

	-debug
		output debugging information while parsing the grammar.
	-h -help
		display this help message.
	-o OUTPUT_FILE
		write the generated parser to OUTPUT_FILE. Defaults to stdout.
	-receiver-name NAME
		use NAME as for the receiver name of the generated methods
		for the grammar's code blocks. Defaults to "c".
	-x
		do not generate the parser, only parse the grammar.

See https://godoc.org/github.com/PuerkitoBio/pigeon for more
information.
`

// usage prints the help page of the command-line tool.
func usage() {
	fmt.Printf(usagePage, os.Args[0])
}

// argError prints an error message to stderr, prints the command usage
// and exits with the specified exit code.
func argError(exit int, msg string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, msg, args...)
	fmt.Fprintln(os.Stderr)
	flag.Usage()
	os.Exit(exit)
}

// input gets the name and reader to get input text from.
func input(filename string) (nm string, rc io.ReadCloser) {
	nm = "stdin"
	inf := os.Stdin
	if filename != "" {
		f, err := os.Open(filename)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
		inf = f
		nm = filename
	}
	r := bufio.NewReader(inf)
	return nm, makeReadCloser(r, inf)
}

// output gets the writer to write the generated parser to.
func output(filename string) io.WriteCloser {
	out := os.Stdout
	if filename != "" {
		f, err := os.Create(filename)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(4)
		}
		out = f
	}
	return out
}

func makeReadCloser(r io.Reader, c io.Closer) io.ReadCloser {
	rc := struct {
		io.Reader
		io.Closer
	}{r, c}
	return io.ReadCloser(rc)
}

// astPos is a helper method for the PEG grammar parser. It returns the
// position of the current match as an ast.Pos.
func (c *current) astPos() ast.Pos {
	return ast.Pos{Line: c.pos.line, Col: c.pos.col, Off: c.pos.offset}
}

// toIfaceSlice is a helper function for the PEG grammar parser. It converts
// v to a slice of empty interfaces.
func toIfaceSlice(v interface{}) []interface{} {
	if v == nil {
		return nil
	}
	return v.([]interface{})
}
