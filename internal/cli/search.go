package cli

import (
	"fmt"

	"memdroid/internal/app"
	"memdroid/internal/driver"
	"memdroid/internal/memory/search"
)

// MaxCandidatesDisplay caps how many search candidates are printed.
const MaxCandidatesDisplay = 50

func SetValueType(st *app.State) {
	fmt.Println("Types: 1=int32  2=int64  3=float32  4=float64  5=uint32  6=uint64  7=bytes")
	var vt search.ValueType
	switch Prompt("Type: ") {
	case "1":
		vt = search.TypeInt32
	case "2":
		vt = search.TypeInt64
	case "3":
		vt = search.TypeFloat32
	case "4":
		vt = search.TypeFloat64
	case "5":
		vt = search.TypeUint32
	case "6":
		vt = search.TypeUint64
	case "7":
		vt = search.TypeBytes
	default:
		fmt.Println("Invalid type")
		return
	}
	st.SetValueType(vt)
	if sess := st.GetSession(); sess != nil {
		st.SetSession(search.NewSession(st.GetPID(), vt, st.GetDriver()))
		fmt.Println("Search session reset for new type")
	}
}

func SearchFiltered(st *app.State) {
	fmt.Println("Region: 1=all  2=heap  3=stack  4=anon  5=custom range")
	var filter driver.RegionFilter
	var customStart, customEnd uintptr
	switch Prompt("Region: ") {
	case "2":
		filter = driver.RegionHeap
	case "3":
		filter = driver.RegionStack
	case "4":
		filter = driver.RegionAnon
	case "5":
		filter = driver.RegionCustom
		var ok bool
		if customStart, ok = ParseAddr("Start address (hex): "); !ok {
			return
		}
		if customEnd, ok = ParseAddr("End address (hex): "); !ok {
			return
		}
	default:
		filter = driver.RegionAll
	}
	val, ok := ParseValue("Value: ", st.GetValueType())
	if !ok {
		return
	}
	if err := st.EnsureSession().SearchFiltered(val, filter, customStart, customEnd); err != nil {
		fmt.Printf("Search failed: %v\n", err)
	}
}

func ShowCandidates(st *app.State) {
	sess := st.GetSession()
	vt := st.GetValueType()
	candidates := sess.Snapshot()
	if len(candidates) == 0 {
		fmt.Println("No candidates")
		return
	}
	shown := 0
	for addr, val := range candidates {
		fmt.Printf("  0x%x = %s\n", addr, search.FormatValue(val, vt))
		shown++
		if shown >= MaxCandidatesDisplay {
			fmt.Printf("  ... (%d total)\n", len(candidates))
			break
		}
	}
}

func searchValue(c *cmd) {
	val, ok := ParseValue("Value: ", c.vt)
	if !ok {
		return
	}
	sess := c.st.EnsureSession()
	if err := sess.Search(val); err != nil {
		fmt.Printf("Search failed: %v\n", err)
		return
	}
	fmt.Printf("Found %d addresses\n", sess.CandidateCount())
}

// filterAction builds the action for one of the no-argument filter modes.
func filterAction(mode search.FilterMode) action {
	return func(c *cmd) {
		if err := c.sess.Filter(mode, nil); err != nil {
			fmt.Printf("Filter failed: %v\n", err)
			return
		}
		fmt.Printf("Remaining: %d addresses\n", c.sess.CandidateCount())
	}
}

func filterByValue(c *cmd) {
	val, ok := ParseValue("Value: ", c.vt)
	if !ok {
		return
	}
	if err := c.sess.Filter(search.FilterValue, val); err != nil {
		fmt.Printf("Filter failed: %v\n", err)
		return
	}
	fmt.Printf("Remaining: %d addresses\n", c.sess.CandidateCount())
}

func resetSearch(c *cmd) {
	if c.sess != nil {
		c.sess.Reset()
	}
	fmt.Println("Search session reset")
}

func searchByPattern(c *cmd) {
	pat, err := search.ParsePattern(Prompt("Pattern (e.g. FF 00 ?? 01): "))
	if err != nil {
		fmt.Printf("Invalid pattern: %v\n", err)
		return
	}
	result, err := search.SearchPattern(c.drv, c.pid, pat)
	if err != nil {
		fmt.Printf("Pattern search failed: %v\n", err)
		return
	}
	printScanResult(result)
}

func searchByString(enc search.StringEncoding) action {
	label := "String (UTF-8): "
	if enc == search.EncodingUTF16LE {
		label = "String (UTF-16LE): "
	}
	return func(c *cmd) {
		result, err := search.SearchString(c.drv, c.pid, Prompt(label), enc)
		if err != nil {
			fmt.Printf("Search failed: %v\n", err)
			return
		}
		printScanResult(result)
	}
}

func printScanResult(result search.PatternResult) {
	for _, m := range result.Matches {
		fmt.Printf("  Found at 0x%x\n", m.Addr)
	}
	fmt.Printf("Total: %d results\n", len(result.Matches))
	if result.Truncated {
		fmt.Printf("(stopped at the %d-result limit — narrow the pattern for more)\n", search.PatternMaxResults)
	}
}
