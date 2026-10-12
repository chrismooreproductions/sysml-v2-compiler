// SPDX-License-Identifier: EPL-2.0

package semantics

import (
	"sort"
	"strings"
	"testing"

	"github.com/chrismooreproductions/sysml-v2-compiler/internal/model"
)

// impliedOf lists the implied general types of the first element of a class
// named name, as "Kind Qualified::Name".
func impliedOf(t *testing.T, w *Workspace, root *model.Element, class, name string) []string {
	t.Helper()
	var target *model.Element
	root.Walk(func(e *model.Element) {
		if target == nil && e.Class == class && e.Text("declaredName") == name {
			target = e
		}
	})
	if target == nil {
		t.Fatalf("no %s named %s", class, name)
	}
	var out []string
	for _, im := range w.ImpliedGeneralizations(target) {
		out = append(out, im.Kind+" "+QualifiedName(im.Target))
	}
	sort.Strings(out)
	return out
}

func TestImpliedGeneralTypes(t *testing.T) {
	w, roots := resolved(t, true, `package P {
		part def Car;
		item def Box { item inner : Box; connection link connect inner to inner; }
		part car : Car;
	}`)
	cases := []struct {
		class, name string
		want        []string
	}{
		{"PartUsage", "car", []string{"Subsetting Parts::parts"}},
		{"ItemUsage", "inner", []string{"Subsetting Items::Item::subitems"}},
		{"ConnectionUsage", "link", []string{"Subsetting Connections::binaryConnections", "Subsetting Items::Item::subparts"}},
	}
	for _, c := range cases {
		got := impliedOf(t, w, roots[0], c.class, c.name)
		if strings.Join(got, "; ") != strings.Join(c.want, "; ") {
			t.Errorf("%s %s: implied %q, want %q", c.class, c.name, got, c.want)
		}
	}
}
