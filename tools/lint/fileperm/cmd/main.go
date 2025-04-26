// Command fileperm-lint checks for hardcoded file permissions
package main

import (
	"github.com/lucas-albers-lz4/irr/tools/lint/fileperm"
	"golang.org/x/tools/go/analysis/singlechecker"
)

func main() {
	singlechecker.Main(fileperm.Analyzer)
}
