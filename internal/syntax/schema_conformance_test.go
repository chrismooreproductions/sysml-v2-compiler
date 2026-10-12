// SPDX-License-Identifier: EPL-2.0

package syntax

import (
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/chrismooreproductions/sysml-v2-compiler/internal/grammar"
	"github.com/chrismooreproductions/sysml-v2-compiler/internal/schema"
)

// Every node the parser builds for the corpus uses a metaclass and properties
// that the specification's metamodel defines, and every value has the kind
// its property declares.
func TestParsedTreesConformToTheSchema(t *testing.T) {
	problems := map[string]int{}
	nodes, files := 0, 0
	check := func(n *Node) {}
	check = func(n *Node) {
		nodes++
		if _, ok := schema.Lookup(n.Class); !ok {
			problems["unknown metaclass "+n.Class]++
			return
		}
		for _, p := range n.Props {
			f, _, ok := schema.FeatureOf(n.Class, p.Name)
			if !ok {
				problems["no property "+n.Class+"."+p.Name]++
				continue
			}
			switch v := p.Value.(type) {
			case *Node:
				if f.Attribute {
					problems["node assigned to attribute "+n.Class+"."+p.Name]++
				}
				if !schema.IsKindOf(v.Class, f.Type) {
					problems["wrong type "+v.Class+" for "+n.Class+"."+p.Name+" ("+f.Type+")"]++
				}
				check(v)
			case Ref:
				if f.Attribute {
					problems["reference assigned to attribute "+n.Class+"."+p.Name]++
				}
			case Bool:
				if f.Type != "EBoolean" {
					problems["boolean assigned to "+n.Class+"."+p.Name+" ("+f.Type+")"]++
				}
			case Text:
				if e, ok := schema.LookupEnum(f.Type); ok {
					if !e.HasLiteral(string(v)) {
						problems["bad literal "+string(v)+" for "+f.Type]++
					}
				} else if !f.Attribute {
					problems["text assigned to reference "+n.Class+"."+p.Name]++
				}
			}
		}
	}
	for _, root := range []string{"../library/sysml.library", "../../testdata/upstream"} {
		filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() {
				return nil
			}
			var lang grammar.Language
			switch filepath.Ext(path) {
			case ".sysml":
				lang = grammar.SysML
			case ".kerml":
				lang = grammar.KerML
			default:
				return nil
			}
			src, _ := os.ReadFile(path)
			n, err := Parse(lang, path, string(src))
			if err != nil {
				t.Errorf("%v", err)
				return nil
			}
			files++
			check(n)
			return nil
		})
	}
	var keys []string
	for k := range problems {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		t.Errorf("%6d  %s", problems[k], k)
	}
	if files < 400 || nodes < 100000 {
		t.Errorf("only %d files and %d nodes checked", files, nodes)
	}
	t.Logf("%d nodes in %d files conform", nodes, files)
}
