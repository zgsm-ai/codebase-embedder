package embedding

import (
	"fmt"
	"slices"
	"strings"

	"github.com/tiktoken-go/tokenizer"
	sitter "github.com/tree-sitter/go-tree-sitter"
	"github.com/zgsm-ai/codebase-indexer/internal/parser"
	"github.com/zgsm-ai/codebase-indexer/internal/types"
)

type CodeSplitter struct {
	tokenizer    tokenizer.Codec
	splitOptions SplitOptions
}

type SplitOptions struct {
	MaxTokensPerChunk          int
	SlidingWindowOverlapTokens int
	EnableMarkdownParsing      bool // 是否启用markdown文件解析
}

// NewCodeSplitter 创建代码分割器
func NewCodeSplitter(splitOptions SplitOptions) (*CodeSplitter, error) {
	codec, err := tokenizer.Get(tokenizer.Cl100kBase)
	if err != nil {
		return nil, fmt.Errorf("failed to get tokenizer: %w", err)
	}

	return &CodeSplitter{
		tokenizer:    codec,
		splitOptions: splitOptions,
	}, nil
}

// Split 将代码文件分割成多个代码块
func (p *CodeSplitter) Split(codeFile *types.SourceFile) ([]*types.CodeChunk, error) {

	language, err := parser.GetLangConfigByFilePath(codeFile.Path)
	if err != nil {
		return nil, err
	}

	if language.Language == parser.Markdown && !p.splitOptions.EnableMarkdownParsing {
		return nil, fmt.Errorf("mardownfile parse is close")
	}

	// 特殊处理 markdown 文件 - 只有在配置开启时才解析markdown
	if language.Language == parser.Markdown && p.splitOptions.EnableMarkdownParsing {
		return p.splitMarkdownFile(codeFile)
	}

	sitterParser := sitter.NewParser()

	// 设置解析器语言（复用已创建的Parser）
	if err := sitterParser.SetLanguage(language.SitterLanguage()); err != nil {
		return nil, fmt.Errorf("failed to set parser language: %w", err)
	}

	// 解析代码
	tree := sitterParser.Parse(codeFile.Content, nil)
	if tree == nil {
		return nil, fmt.Errorf("failed to parse code: %s", codeFile.Path)
	}
	defer tree.Close()

	// 获取要提取的节点类型
	nodeKinds, ok := languageChunkNodeKind[language.Language]
	if !ok {
		return nil, fmt.Errorf("missing chunk config for language %s", language.Language)
	}

	// 预分配切片，减少内存重新分配
	estimatedChunks := 10 // 预估每个文件约10个代码块
	allChunks := make([]*types.CodeChunk, 0, estimatedChunks)

	// 遍历语法树
	cursor := tree.RootNode().Walk()
	defer cursor.Close()

	// 使用更简洁的遍历逻辑
	for {
		currentNode := cursor.Node()
		kind := currentNode.Kind()
		// 处理目标节点类型
		if slices.Contains(nodeKinds, kind) {
			// 提取节点信息
			startPos := currentNode.StartPosition()
			endPos := currentNode.EndPosition()
			content := codeFile.Content[currentNode.StartByte():currentNode.EndByte()]
			tokenCount := p.countToken(content)

			// 处理代码切块
			if tokenCount > p.splitOptions.MaxTokensPerChunk {
				subChunks := p.splitFuncWithSlidingWindow(string(content), codeFile, int(startPos.Row))
				allChunks = append(allChunks, subChunks...)
			} else {
				allChunks = append(allChunks, &types.CodeChunk{
					Language:     "code",
					CodebaseId:   codeFile.CodebaseId,
					CodebasePath: codeFile.CodebasePath,
					CodebaseName: codeFile.CodebaseName,
					Content:      content,
					FilePath:     codeFile.Path,
					Range:        []int{int(startPos.Row), int(startPos.Column), int(endPos.Row), int(endPos.Column)},
					TokenCount:   tokenCount,
				})
			}

			// 跳过子节点，直接移动到兄弟节点
			if !cursor.GotoNextSibling() {
				// 没有兄弟节点，回溯到父节点的兄弟节点
				for {
					if !cursor.GotoParent() {
						return allChunks, nil // 遍历完成
					}
					if cursor.GotoNextSibling() {
						break
					}
				}
			}
			continue
		}

		// 非目标节点，继续深度优先遍历
		if cursor.GotoFirstChild() {
			continue
		}

		// 无子节点，尝试兄弟节点
		for {
			if cursor.GotoNextSibling() {
				break
			}

			// 无兄弟节点，回溯父节点
			if !cursor.GotoParent() {
				return allChunks, nil // 遍历完成
			}
		}
	}
}

// countToken 计算内容的token数量
func (p *CodeSplitter) countToken(content []byte) int {
	// 避免不必要的字符串转换
	contentStr := string(content)
	tokenCount, err := p.tokenizer.Count(contentStr)
	if err != nil {
		// 回退到简单的长度计算
		return len(contentStr) / 4 // 粗略估计：1token≈4字符
	}
	return tokenCount
}

// splitFuncWithSlidingWindow 使用滑动窗口将大函数分割成多个小块
func (p *CodeSplitter) splitFuncWithSlidingWindow(content string, codeFile *types.SourceFile, funcStartLine int) []*types.CodeChunk {
	filePath := codeFile.Path
	maxTokens := p.splitOptions.MaxTokensPerChunk
	overlapTokens := p.splitOptions.SlidingWindowOverlapTokens

	if maxTokens <= 0 || overlapTokens < 0 || overlapTokens >= maxTokens {
		return nil
	}

	// 编码内容获取tokens和字节偏移量
	_, tokens, err := p.tokenizer.Encode(content)
	if err != nil {
		return nil
	}

	totalTokens := len(tokens)
	if totalTokens == 0 {
		return nil
	}

	// 计算每个token的字节偏移量
	byteOffsets := make([]int, len(tokens)+1)
	currentOffset := 0
	for i, token := range tokens {
		byteOffsets[i] = currentOffset
		currentOffset += len(token)
	}
	byteOffsets[len(tokens)] = currentOffset

	// 预分配切片
	estimatedChunks := (totalTokens + maxTokens - 1) / maxTokens
	chunks := make([]*types.CodeChunk, 0, estimatedChunks)

	startTokenIdx := 0

	for startTokenIdx < totalTokens {
		// 计算当前块的结束位置
		endTokenIdx := startTokenIdx + maxTokens
		if endTokenIdx > totalTokens {
			endTokenIdx = totalTokens
		}

		// 提取代码块
		startByte := byteOffsets[startTokenIdx]
		endByte := byteOffsets[endTokenIdx] - 1
		if endByte >= len(content) {
			endByte = len(content) - 1
		}

		chunkContent := content[startByte : endByte+1]

		// 计算起始行和列
		startLine := funcStartLine + countLines(content[:startByte])
		startColumn := calculateColumn(content, startByte)

		// 计算结束行和列
		endLine := startLine + countLines(chunkContent) - 1
		endColumn := calculateColumn(content[startByte:endByte+1], endByte-startByte)

		chunks = append(chunks, &types.CodeChunk{
			Language:     "code",
			CodebaseId:   codeFile.CodebaseId,
			CodebasePath: codeFile.CodebasePath,
			CodebaseName: codeFile.CodebaseName,
			Content:      []byte(chunkContent),
			FilePath:     filePath,
			Range:        []int{startLine, startColumn, endLine, endColumn},
			TokenCount:   endTokenIdx - startTokenIdx,
		})

		if endTokenIdx >= totalTokens {
			break
		}

		// 计算下一个块的起始位置（应用滑动窗口）
		if remaining := totalTokens - endTokenIdx; remaining < maxTokens {
			// 最后一块，调整重叠量
			startTokenIdx = endTokenIdx - (maxTokens - remaining)
		} else {
			// 正常情况，使用固定重叠
			startTokenIdx = endTokenIdx - overlapTokens
		}

		// 防止索引越界
		if startTokenIdx < 0 {
			startTokenIdx = 0
		}
	}

	return chunks
}

// calculateColumn 根据字节偏移量计算在当前行的列位置
func calculateColumn(content string, byteOffset int) int {
	if byteOffset >= len(content) {
		byteOffset = len(content) - 1
	}
	if byteOffset < 0 {
		return 0
	}

	// 从字节偏移量向前查找最后一个换行符
	column := 0
	for i := byteOffset; i >= 0; i-- {
		if content[i] == '\n' {
			break
		}
		column++
	}
	return column
}

// countLines 计算字符串中的行数
func countLines(s string) int {
	if len(s) == 0 {
		return 0
	}

	count := 0
	for _, c := range s {
		if c == '\n' {
			count++
		}
	}

	// 最后一行可能没有换行符
	if len(s) > 0 && s[len(s)-1] != '\n' {
		count++
	}

	return count
}

// splitMarkdownFile 将 markdown 文件分割成多个代码块
func (p *CodeSplitter) splitMarkdownFile(codeFile *types.SourceFile) ([]*types.CodeChunk, error) {
	content := string(codeFile.Content)
	lines := strings.Split(content, "\n")

	var chunks []*types.CodeChunk
	var currentChunk strings.Builder
	var currentLine int
	var inCodeBlock bool

	for i, line := range lines {
		// 检查是否是代码块开始或结束
		if strings.HasPrefix(line, "```") {
			if inCodeBlock {
				// 代码块结束
				currentChunk.WriteString(line + "\n")
				chunkContent := currentChunk.String()
				tokenCount := p.countToken([]byte(chunkContent))

				chunks = append(chunks, &types.CodeChunk{
					Language:     "markdown",
					CodebaseId:   codeFile.CodebaseId,
					CodebasePath: codeFile.CodebasePath,
					CodebaseName: codeFile.CodebaseName,
					Content:      []byte(chunkContent),
					FilePath:     codeFile.Path,
					Range:        []int{currentLine, 0, i, len(line)},
					TokenCount:   tokenCount,
				})

				currentChunk.Reset()
				inCodeBlock = false
			} else {
				// 代码块开始，先保存之前的内容
				if currentChunk.Len() > 0 {
					chunkContent := currentChunk.String()
					tokenCount := p.countToken([]byte(chunkContent))

					chunks = append(chunks, &types.CodeChunk{
						Language:     "markdown",
						CodebaseId:   codeFile.CodebaseId,
						CodebasePath: codeFile.CodebasePath,
						CodebaseName: codeFile.CodebaseName,
						Content:      []byte(chunkContent),
						FilePath:     codeFile.Path,
						Range:        []int{currentLine, 0, i - 1, len(lines[i-1])},
						TokenCount:   tokenCount,
					})

					currentChunk.Reset()
				}

				currentChunk.WriteString(line + "\n")
				currentLine = i
				inCodeBlock = true
			}
			continue
		}

		// 检查是否是标题（# ## ### 等）
		if !inCodeBlock && strings.HasPrefix(line, "#") {
			// 保存之前的内容
			if currentChunk.Len() > 0 {
				chunkContent := currentChunk.String()
				tokenCount := p.countToken([]byte(chunkContent))

				chunks = append(chunks, &types.CodeChunk{
					Language:     "markdown",
					CodebaseId:   codeFile.CodebaseId,
					CodebasePath: codeFile.CodebasePath,
					CodebaseName: codeFile.CodebaseName,
					Content:      []byte(chunkContent),
					FilePath:     codeFile.Path,
					Range:        []int{currentLine, 0, i - 1, len(lines[i-1])},
					TokenCount:   tokenCount,
				})

				currentChunk.Reset()
			}

			currentChunk.WriteString(line + "\n")
			currentLine = i
			continue
		}

		// 普通内容
		currentChunk.WriteString(line + "\n")

		// 检查当前块是否超过最大 token 数量
		if currentChunk.Len() > 0 {
			tokenCount := p.countToken([]byte(currentChunk.String()))
			if tokenCount > p.splitOptions.MaxTokensPerChunk {
				chunkContent := currentChunk.String()
				subChunks := p.splitFuncWithSlidingWindow(chunkContent, codeFile, currentLine)
				chunks = append(chunks, subChunks...)
				currentChunk.Reset()
				currentLine = i + 1
			}
		}
	}

	// 添加最后一块内容
	if currentChunk.Len() > 0 {
		chunkContent := currentChunk.String()
		tokenCount := p.countToken([]byte(chunkContent))

		chunks = append(chunks, &types.CodeChunk{
			Language:     "markdown",
			CodebaseId:   codeFile.CodebaseId,
			CodebasePath: codeFile.CodebasePath,
			CodebaseName: codeFile.CodebaseName,
			Content:      []byte(chunkContent),
			FilePath:     codeFile.Path,
			Range:        []int{currentLine, 0, len(lines) - 1, len(lines[len(lines)-1])},
			TokenCount:   tokenCount,
		})
	}

	return chunks, nil
}
