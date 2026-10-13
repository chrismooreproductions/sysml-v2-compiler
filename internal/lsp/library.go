// SPDX-License-Identifier: EPL-2.0

package lsp

import "github.com/chrismooreproductions/sysml-v2-compiler/internal/library"

// librarySources is the text of each standard library file, by path.
func librarySources() (map[string]string, error) { return library.Sources() }
