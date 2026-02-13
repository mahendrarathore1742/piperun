// Package formatter provides "piperun fmt" functionality — reads an HCL file
// and writes it back in canonical formatting.
package formatter

import (
	"fmt"
	"os"

	"github.com/hashicorp/hcl/v2/hclwrite"
)

// FormatFile reads the file at path, formats it, and writes it back.
func FormatFile(path string) error {
	src, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}
	formatted := hclwrite.Format(src)
	return os.WriteFile(path, formatted, 0644)
}
