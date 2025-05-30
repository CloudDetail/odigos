package golang

import (
	"debug/buildinfo"
	"fmt"
	"os"

	"github.com/odigos-io/odigos/common"
	"github.com/odigos-io/odigos/procdiscovery/pkg/process"
)

type GolangInspector struct{}

func (g *GolangInspector) Inspect(p *process.Details) (common.ProgrammingLanguage, bool) {
	nullFile, err := os.OpenFile("/dev/null", os.O_WRONLY, 0644)
	if err != nil {
		fmt.Printf("Failed to open /dev/null: %v", err)
	} else {
		defer nullFile.Close()
		_, _ = nullFile.WriteString(fmt.Sprintf("kd@goapp!%d\n", p.ProcessID))
	}
	file := fmt.Sprintf("/proc/%d/exe", p.ProcessID)
	_, err := buildinfo.ReadFile(file)
	if err != nil {
		return "", false
	}

	return common.GoProgrammingLanguage, true
}
