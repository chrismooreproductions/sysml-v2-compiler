// SPDX-License-Identifier: EPL-2.0

package lsp

import (
	"sort"
	"sync"

	"github.com/chrismooreproductions/sysml-v2-compiler/internal/grammar"
)

var (
	keywordsOnce sync.Once
	keywordList  []string
)

// keywords lists the reserved words of SysML, sorted.
func keywords() []string {
	keywordsOnce.Do(func() {
		if g, err := grammar.Load(grammar.SysML); err == nil {
			for k := range g.Keywords {
				keywordList = append(keywordList, k)
			}
			sort.Strings(keywordList)
		}
	})
	return keywordList
}
