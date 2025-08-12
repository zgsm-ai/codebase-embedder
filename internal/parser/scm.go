package parser

import (
	"embed"
	"fmt"
	"path/filepath"

	sitter "github.com/tree-sitter/go-tree-sitter"
)

//go:embed queries/*/*.scm
var scmFS embed.FS

const queryDir = "queries"
const defSubdir = "def"
const baseSubDir = "base"
const queryExt = ".scm"

var DefinitionQueries = make(map[Language]string)
var BaseQueries = make(map[Language]string)

func init() {
	if err := loadScm(); err != nil {
		panic(fmt.Errorf("tree_sitter parser load scm queries err:%v", err))
	}
}

func loadScm() error {
	configs := GetLanguageConfigs()
	for _, lang := range configs {
		// 校验query
		langParser := sitter.NewParser()
		sitterLang := lang.SitterLanguage()
		// 检查语言是否为nil（例如Markdown语言没有对应的tree-sitter解析器）
		if sitterLang == nil {
			// 对于不支持的语言，跳过查询加载
			langParser.Close()
			continue
		}
		err := langParser.SetLanguage(sitterLang)
		if err != nil {
			return fmt.Errorf("failed to init language parser %s: %w", lang.Language, err)
		}

		baseQueryContent, err := loadLanguageScm(lang, baseSubDir, sitterLang)
		if err != nil {
			return err
		}

		defQueryContent, err := loadLanguageScm(lang, defSubdir, sitterLang)
		if err != nil {
			return err
		}

		langParser.Close()
		BaseQueries[lang.Language] = string(baseQueryContent)
		DefinitionQueries[lang.Language] = string(defQueryContent)
	}
	return nil
}

func loadLanguageScm(lang *LanguageConfig, scmDir string, sitterLang *sitter.Language) ([]byte, error) {
	var err error
	baseQuery := makeQueryPath(lang.Language, scmDir)
	baseQueryContent, err := scmFS.ReadFile(baseQuery)
	if err != nil {
		return nil, fmt.Errorf("failed to read base query file %s for %s: %w", baseQuery, lang.Language, err)
	}
	query, queryError := sitter.NewQuery(sitterLang, string(baseQueryContent))
	if queryError != nil && IsRealQueryErr(queryError) {
		return nil, fmt.Errorf("failed to parse base query file %s: %w", baseQuery, queryError)
	}
	query.Close()
	return baseQueryContent, nil
}

func makeQueryPath(lang Language, subdir string) string {
	return filepath.ToSlash(filepath.Join(queryDir, subdir, string(lang)+queryExt))
}
