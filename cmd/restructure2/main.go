// The restructure tool recovers control flow primitives from control flow
// graphs (*.dot -> *.json).
//
// The input of restructure is a Graphviz DOT file, containing the unstructured
// control flow graph of a function, and the output is a JSON stream describing
// how the recovered high-level control flow primitives relate to the nodes of
// the control flow graph.
//
// Usage:
//
//     restructure [OPTION]... [FILE.dot]
//
// Flags:
//
//   -img
//         output image representation of graphs
//   -indent
//         indent JSON output
//   -method string
//         control flow recovery method (hammock, interval, pattern-independent)
//         (default "hammock")
//   -o string
//         output path
//   -q    suppress non-error messages
//   -steps
//         output intermediate steps
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"strconv"

	"github.com/mewkiz/pkg/pathutil"
	"github.com/mewkiz/pkg/term"
	"github.com/mewmew/lnp/pkg/cfa"
	"github.com/mewmew/lnp/pkg/cfa/hammock"
	"github.com/mewmew/lnp/pkg/cfa/interval"
	"github.com/mewmew/lnp/pkg/cfa/primitive"
	"github.com/mewmew/lnp/pkg/cfg"
	"github.com/pkg/errors"
	"gonum.org/v1/gonum/graph/encoding"
)

var (
	// dbg is a logger which logs debug messages to standard error, prepending
	// the "restructure:" prefix.
	dbg = log.New(os.Stderr, term.MagentaBold("restructure:")+" ", 0)
	// warn is a logger which logs warning messages to standard error, prepending
	// the "restructure:" prefix.
	warn = log.New(os.Stderr, term.RedBold("restructure:")+" ", 0)
)

func usage() {
	const use = `
Recover control flow primitives from control flow graphs (*.dot -> *.json).

Usage:

	restructure [OPTION]... [FILE.dot]

Flags:
`
	fmt.Fprintln(os.Stderr, use[1:])
	flag.PrintDefaults()
}

func main() {
	// Parse command line arguments.
	var (
		// img specifies whether to output image representation of graphs.
		img bool
		// indent specifies whether to indent JSON output.
		indent bool
		// method specifies the control flow recovery method (hammock, interval,
		// pattern-independent).
		method string
		// output specifies the output path.
		output string
		// quiet specifies whether to suppress non-error messages.
		quiet bool
		// steps specifies whether to output intermediate steps.
		steps bool
	)
	flag.BoolVar(&img, "img", false, "output image representation of graphs")
	flag.BoolVar(&indent, "indent", false, "indent JSON output")
	flag.StringVar(&method, "method", "hammock", "control flow recovery method (hammock, interval, pattern-independent)")
	flag.StringVar(&output, "o", "", "output path")
	flag.BoolVar(&quiet, "q", false, "suppress non-error messages")
	flag.BoolVar(&steps, "steps", false, "output intermediate steps")
	flag.Usage = usage
	flag.Parse()
	var dotPath string
	switch flag.NArg() {
	case 0:
		// Parse DOT file from standard input.
		dotPath = "-"
	case 1:
		dotPath = flag.Arg(0)
	default:
		flag.Usage()
		os.Exit(1)
	}
	if quiet {
		// Mute debug messages if `-q` is set.
		dbg.SetOutput(ioutil.Discard)
	}

	// Perform control flow analysis.
	prims, err := restructure(dotPath, method, steps, img)
	if err != nil {
		log.Fatalf("%+v", err)
	}

	// Output primitives in JSON format.
	w := os.Stdout
	if len(output) > 0 {
		f, err := os.Create(output)
		if err != nil {
			log.Fatal(err)
		}
		defer f.Close()
		w = f
	}
	if err := outputJSON(w, prims, indent); err != nil {
		log.Fatalf("%+v", err)
	}
}

// restructure attempts to recover the control flow primitives of a given
// control flow graph.
//
// method specifies the control flow recovery method to use.
//
// steps specifies whether to record the intermediate control flow graphs at
// each step. The returned list of primitives is ordered in the same sequence as
// they were located.
//
// img specifies whether to output image representations of the intermediate
// control flow graphs.
func restructure(dotPath, method string, steps, img bool) ([]*primitive.Primitive, error) {
	var stepPrefix string
	switch dotPath {
	case "-":
		// Use "stdin" prefix for intermediate step files.
		stepPrefix = "stdin"
	default:
		stepPrefix = pathutil.TrimExt(dotPath)
	}
	// Output intermediate steps in Graphviz DOT format.
	var (
		before func(g cfa.Graph, prim *primitive.Primitive)
		after  func(g cfa.Graph, prim *primitive.Primitive)
	)
	step := 1
	if steps {
		before = func(g cfa.Graph, prim *primitive.Primitive) {
			data := []byte(dotBeforeMerge(g, prim))
			dbg.Printf("located primitive:\n%s", prim)
			beforePath := fmt.Sprintf("%s_%04da.dot", stepPrefix, step)
			dbg.Printf("creating file %q", beforePath)
			if err := ioutil.WriteFile(beforePath, data, 0644); err != nil {
				warn.Printf("unable to create %q; %v", beforePath, err)
			}
			// Store an image representation of the intermediate CFG if `-img` is
			// set.
			if img {
				if err := outputImg(beforePath); err != nil {
					warn.Println(err)
				}
			}
		}
		after = func(g cfa.Graph, prim *primitive.Primitive) {
			data := []byte(dotAfterMerge(g, prim))
			afterPath := fmt.Sprintf("%s_%04db.dot", stepPrefix, step)
			dbg.Printf("creating file %q", afterPath)
			if err := ioutil.WriteFile(afterPath, data, 0644); err != nil {
				warn.Printf("unable to create %q; %v", afterPath, err)
			}
			// Store an image representation of the intermediate CFG if `-img` is
			// set.
			if img {
				if err := outputImg(afterPath); err != nil {
					warn.Println(err)
				}
			}
			step++
		}
	}
	// Recovery control flow primitives.
	switch method {
	case "hammock":
		// Parse control flow graph.
		g := cfg.NewGraph()
		if err := parseCFGInto(dotPath, g); err != nil {
			return nil, errors.WithStack(err)
		}
		// Perform control flow analysis.
		prims, err := hammock.Analyze(g, before, after)
		if err != nil {
			if errors.Cause(err) == cfa.ErrIncomplete {
				warn.Printf("warning: %v", err)
			} else {
				return nil, errors.WithStack(err)
			}
		}
		return prims, nil
	case "interval":
		// Parse control flow graph.
		g := interval.NewGraph()
		if err := parseCFGInto(dotPath, g); err != nil {
			return nil, errors.WithStack(err)
		}
		// Output derived sequence of graphs.
		if steps {
			Gs, IIs := interval.DerivedSequence(g)
			for i, g := range Gs {
				name := fmt.Sprintf("G_%d.dot", i+1)
				if err := ioutil.WriteFile(name, []byte(g.String()), 0644); err != nil {
					return nil, errors.WithStack(err)
				}
			}
			for i, Is := range IIs {
				for j, I := range Is {
					name := fmt.Sprintf("I_%d_%d.dot", i+1, j+1)
					if err := ioutil.WriteFile(name, []byte(I.String()), 0644); err != nil {
						return nil, errors.WithStack(err)
					}
				}
			}
		}
		// Perform control flow analysis.
		prims := interval.Analyze(g, before, after)
		return prims, nil
	default:
		panic(fmt.Errorf("support for control flow recovery method %q not yet implemented", method))
	}
}

// dotBeforeMerge returns the intermediate graph g in Graphviz DOT format with
// nodes before merge highlighted in red that are part of the located primitive.
func dotBeforeMerge(g cfa.Graph, prim *primitive.Primitive) string {
	// Colour nodes red.
	for _, dotID := range prim.Nodes {
		n, ok := g.NodeWithDOTID(dotID)
		if !ok {
			panic(fmt.Errorf("unable to locate node %q in control flow graph", dotID))
		}
		setFillColor(n, "red")
		if dotID == prim.Entry {
			// Add an external label with the name of the primitive for the entry
			// node of the primitive.
			// TODO: investigate whether quoting of attributes should be done by
			// gonum/encoding/dot.
			n.SetAttribute(encoding.Attribute{Key: "xlabel", Value: strconv.Quote(prim.Prim)})
		}
	}
	s := g.String()
	// Restore node colour.
	for _, dotID := range prim.Nodes {
		n, ok := g.NodeWithDOTID(dotID)
		if !ok {
			panic(fmt.Errorf("unable to locate node %q in control flow graph", dotID))
		}
		clearFillColor(n)
		if dotID == prim.Entry {
			// Clear external label from entry node of primitive.
			n.DelAttribute("xlabel")
		}
	}
	return s
}

// dotAfterMerge returns the intermediate graph g in Graphviz DOT format with
// the new node after merge highlighted in red that is part of the located
// primitive.
func dotAfterMerge(g cfa.Graph, prim *primitive.Primitive) string {
	// Colour nodes red.
	n, ok := g.NodeWithDOTID(prim.Entry)
	if !ok {
		panic(fmt.Errorf("unable to locate node %q in control flow graph", prim.Entry))
	}
	setFillColor(n, "red")
	s := g.String()
	// Restore node colour.
	clearFillColor(n)
	return s
}

// setFillColor sets the fillcolor attributes of the node to the given colour.
// The style attribute is also set to filled.
func setFillColor(n cfa.Node, color string) {
	n.SetAttribute(encoding.Attribute{Key: "fillcolor", Value: color})
	n.SetAttribute(encoding.Attribute{Key: "style", Value: "filled"})
}

// clearFillColor clears the fillcolor attribute of the node. The style
// attribute is also cleared.
func clearFillColor(n cfa.Node) {
	n.DelAttribute("fillcolor")
	n.DelAttribute("style")
}

// parseCFGInto parses the given control flow graph in Graphviz DOT format into
// dst.
func parseCFGInto(dotPath string, dst cfa.Graph) error {
	switch dotPath {
	case "-":
		// Read from standard input.
		return cfg.ParseInto(os.Stdin, dst)
	default:
		return cfg.ParseFileInto(dotPath, dst)
	}
}

// outputJSON outputs the primitives in JSON format with optional indentation,
// writing to w.
func outputJSON(w io.Writer, prims []*primitive.Primitive, indent bool) error {
	// Output indented JSON.
	if indent {
		buf, err := json.MarshalIndent(prims, "", "\t")
		if err != nil {
			return errors.WithStack(err)
		}
		buf = append(buf, '\n')
		if _, err := io.Copy(w, bytes.NewReader(buf)); err != nil {
			return errors.WithStack(err)
		}
		return nil
	}
	// Output JSON.
	enc := json.NewEncoder(w)
	if err := enc.Encode(prims); err != nil {
		return errors.WithStack(err)
	}
	return nil
}

// outputImg outputs an image representation of the given Graphviz DOT file.
func outputImg(dotPath string) error {
	pngPath := pathutil.TrimExt(dotPath) + ".png"
	dbg.Printf("creating file %q", pngPath)
	cmd := exec.Command("dot", "-Tpng", "-o", pngPath, dotPath)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return errors.WithStack(err)
	}
	return nil
}
