package skill

import (
	"fmt"
	"io"
	"strings"

	"CleanCaregent/internal/diagnosis"
	"CleanCaregent/internal/generator"
	"CleanCaregent/internal/intent"
	"CleanCaregent/internal/memory"
	"CleanCaregent/internal/rag"
	"CleanCaregent/internal/tool"
	"github.com/spf13/viper"
)

type Definition struct {
	Name          string         `mapstructure:"name"`
	Enabled       bool           `mapstructure:"enabled"`
	Intents       []string       `mapstructure:"intents"`
	DocTypes      []string       `mapstructure:"doc_types"`
	Retrieval     WorkflowConfig `mapstructure:"retrieval"`
	NextSkill     string         `mapstructure:"next_skill"`
	NextSkillArgs map[string]any `mapstructure:"next_skill_args"`
}

type DefinitionFile struct {
	Skills []Definition `mapstructure:"skills"`
}

type Dependencies struct {
	Retriever      rag.Retriever
	Generator      generator.Generator
	Tools          *tool.Executor
	DiagnosisStore memory.Store
}

func LoadDefinitions(reader io.Reader) ([]Definition, error) {
	config := viper.New()
	config.SetConfigType("yaml")
	if err := config.ReadConfig(reader); err != nil {
		return nil, fmt.Errorf("读取 Skill 配置失败: %w", err)
	}
	var file DefinitionFile
	if err := config.Unmarshal(&file); err != nil {
		return nil, fmt.Errorf("解析 Skill 配置失败: %w", err)
	}
	return file.Skills, nil
}

func BuildConfigured(definitions []Definition, deps Dependencies) ([]Skill, error) {
	result := make([]Skill, 0, len(definitions))
	for _, definition := range definitions {
		if !definition.Enabled {
			continue
		}
		workflow, err := buildConfiguredWorkflow(definition, deps)
		if err != nil {
			return nil, err
		}
		result = append(result, workflow)
	}
	return result, nil
}

func buildConfiguredWorkflow(definition Definition, deps Dependencies) (*Workflow, error) {
	var workflow *Workflow
	switch definition.Name {
	case ProductComparisonSkill:
		workflow = NewProductComparison(deps.Retriever, deps.Generator, deps.Tools, definition.Retrieval)
	case PurchaseRecommendationSkill:
		workflow = NewPurchaseRecommendation(deps.Retriever, deps.Generator, deps.Tools, definition.Retrieval)
	case AccessoryCompatibilitySkill:
		workflow = NewAccessoryCompatibility(deps.Retriever, deps.Generator, deps.Tools, definition.Retrieval)
	case FaultDiagnosisSkill:
		workflow = NewFaultDiagnosis(deps.Retriever, deps.Generator, deps.Tools, definition.Retrieval, deps.DiagnosisStore)
	case AfterSalesJudgementSkill:
		workflow = NewAfterSalesJudgement(deps.Retriever, deps.Generator, deps.Tools, definition.Retrieval)
	default:
		return nil, fmt.Errorf("未知 Skill: %s", definition.Name)
	}
	if len(definition.Intents) > 0 {
		workflow.intents = make(map[intent.Type]struct{}, len(definition.Intents))
		for _, raw := range definition.Intents {
			value := intent.Type(strings.TrimSpace(raw))
			if value == "" {
				return nil, fmt.Errorf("Skill %s 包含空意图", definition.Name)
			}
			workflow.intents[value] = struct{}{}
		}
	}
	workflow.docTypes = append([]string(nil), definition.DocTypes...)
	workflow.nextSkill = strings.TrimSpace(definition.NextSkill)
	workflow.nextSkillArgs = cloneAnyMap(definition.NextSkillArgs)
	if workflow.name == FaultDiagnosisSkill && workflow.diagnosisEngine == nil {
		workflow.diagnosisEngine = diagnosis.NewDefaultEngine()
	}
	return workflow, nil
}
