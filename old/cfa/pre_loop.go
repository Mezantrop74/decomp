package cfa

import (
	"fmt"

	"github.com/decomp/decomp/cfa/primitive"
	"github.com/decomp/decomp/graph/cfg"
	"gonum.org/v1/gonum/graph"
)

// PreLoop represents a pre-test loop.
//
// Pseudo-code:
//
//    while (A) {
//       B
//    }
//    C
type PreLoop struct {
	// Condition node (A).
	Cond graph.Node
	// Body node (B).
	Body graph.Node
	// Exit node (C).
	Exit graph.Node
}

// Prim returns a representation of the high-level control flow primitive, as a
// mapping from control flow primitive node names to control flow graph node
// names.
//
// Example mapping:
//
//    "cond": "A"
//    "body": "B"
//    "exit": "C"
func (prim PreLoop) Prim() *primitive.Primitive {
	cond, body, exit := label(prim.Cond), label(prim.Body), label(prim.Exit)
	return &primitive.Primitive{
		Prim: "pre_loop",
		Nodes: map[string]string{
			"cond": cond,
			"body": body,
			"exit": exit,
		},
		Entry: cond,
		Exit:  exit,
	}
}

// String returns a string representation of prim in DOT format.
//
// Example output:
//
//    digraph pre_loop {
//       cond -> body
//       cond -> exit
//       body -> cond
//    }
func (prim PreLoop) String() string {
	cond, body, exit := label(prim.Cond), label(prim.Body), label(prim.Exit)
	const format = `
digraph pre_loop {
	%v -> %v
	%v -> %v
	%v -> %v
}`
	return fmt.Sprintf(format[1:], cond, body, cond, exit, body, cond)
}

// FindPreLoop returns the first occurrence of a pre-test loop in g, and a
// boolean indicating if such a primitive was found.
func FindPreLoop(g graph.Directed, dom cfg.DominatorTree) (prim PreLoop, ok bool) {
	// Range through cond node candidates.
	condNodes := g.Nodes()
	for condNodes.Next() {
		cond := condNodes.Node()
		// Verify that cond has two successors (body and exit).
		condSuccs := graph.NodesOf(g.From(cond.ID()))
		if len(condSuccs) != 2 {
			continue
		}
		prim.Cond = cond

		// Select body and exit node candidates.
		prim.Body, prim.Exit = condSuccs[0], condSuccs[1]
		if prim.IsValid(g, dom) {
			return prim, true
		}

		// Swap body and exit node candidates and try again.
		prim.Body, prim.Exit = prim.Exit, prim.Body
		if prim.IsValid(g, dom) {
			return prim, true
		}
	}
	return PreLoop{}, false
}

// IsValid reports whether the cond, body and exit node candidates of prim form
// a valid pre-test loop in g.
//
// Control flow graph:
//
//    cond
//    ???  ??????
//    ???   body
//    ???
//    exit
func (prim PreLoop) IsValid(g graph.Directed, dom cfg.DominatorTree) bool {
	// Dominator sanity check.
	cond, body, exit := prim.Cond, prim.Body, prim.Exit
	if !dom.Dominates(cond, body) || !dom.Dominates(cond, exit) {
		return false
	}

	// Verify that cond has two successors (body and exit).
	condSuccs := g.From(cond.ID())
	if condSuccs.Len() != 2 || !g.HasEdgeFromTo(cond.ID(), body.ID()) || !g.HasEdgeFromTo(cond.ID(), exit.ID()) {
		return false
	}

	// Verify that body has one predecessor (cond) and one successor (cond).
	bodyPreds := g.To(body.ID())
	bodySuccs := g.From(body.ID())
	if bodyPreds.Len() != 1 || bodySuccs.Len() != 1 || !g.HasEdgeFromTo(body.ID(), cond.ID()) {
		return false
	}

	// Verify that exit has one predecessor (cond).
	exitPreds := g.To(exit.ID())
	return exitPreds.Len() == 1
}
