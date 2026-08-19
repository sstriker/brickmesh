package pipeline

import (
	"fmt"
	"testing"
)

func TestProbeDiff(t *testing.T) {
	deps := requireLibraries(t)
	for _, n := range []string{"62821.dat", "6573.dat"} {
		g, err := deps.Lib.Geometry(n)
		if err != nil {
			fmt.Printf("%s: %v\n", n, err)
			continue
		}
		lo, hi := g.BBox()
		fmt.Printf("%-10s %-40s bbox %v .. %v  size %v\n", n, g.Title, lo, hi, hi.Sub(lo))
		for _, h := range deps.Shadow.Holes(n) {
			fmt.Printf("    port pos=%+v axis=%+v cross=%v\n", h.Pos, h.Axis, h.Cross)
		}
	}
}
