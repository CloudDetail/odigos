package inspectors

import (
	"fmt"

	"github.com/odigos-io/odigos/common"
	"github.com/odigos-io/odigos/procdiscovery/pkg/inspectors/dotnet"
	"github.com/odigos-io/odigos/procdiscovery/pkg/inspectors/golang"
	"github.com/odigos-io/odigos/procdiscovery/pkg/inspectors/java"
	"github.com/odigos-io/odigos/procdiscovery/pkg/inspectors/mysql"
	"github.com/odigos-io/odigos/procdiscovery/pkg/inspectors/nodejs"
	"github.com/odigos-io/odigos/procdiscovery/pkg/inspectors/python"
	"github.com/odigos-io/odigos/procdiscovery/pkg/process"
)

type ErrLanguageDetectionConflict struct {
	languages [2]common.ProgrammingLanguage
}

func (e ErrLanguageDetectionConflict) Error() string {
	return fmt.Sprintf("language detection conflict between %v and %v", e.languages[0], e.languages[1])
}

type inspector interface {
	Inspect(process *process.Details) (common.ProgrammingLanguage, bool)
}

var inspectorsList = []inspector{
	&golang.GolangInspector{},
	&java.JavaInspector{},
	&python.PythonInspector{},
	&dotnet.DotnetInspector{},
	&nodejs.NodejsInspector{},
	&mysql.MySQLInspector{},
}

// DetectLanguage returns the detected language for the process or
// common.UnknownProgrammingLanguage if the language could not be detected, in which case error == nil
// if error or language detectors disagree common.UnknownProgrammingLanguage is also returned
func DetectLanguage(process process.Details) (common.ProgrammingLanguage, error) {
	detectedLanguage := common.UnknownProgrammingLanguage
	for _, i := range inspectorsList {
		language, detected := i.Inspect(&process)
		if detected {
			if detectedLanguage == common.UnknownProgrammingLanguage {
				detectedLanguage = language
				continue
			} else if detectedLanguage == common.DotNetProgrammingLanguage {
				detectedLanguage = language
				continue
			}
			return common.UnknownProgrammingLanguage, ErrLanguageDetectionConflict{
				languages: [2]common.ProgrammingLanguage{
					detectedLanguage,
					language,
				},
			}
		}
	}

	return detectedLanguage, nil
}
