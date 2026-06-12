// Package evaldata embeds the canonical evaluation dataset used by the server.
package evaldata

import _ "embed"

//go:embed eval-cases-v2.json
var casesV2 []byte

// CasesV2 returns an isolated copy of the canonical v2 dataset bytes.
func CasesV2() []byte {
	return append([]byte(nil), casesV2...)
}
