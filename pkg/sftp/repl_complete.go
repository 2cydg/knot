package sftp

import (
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/chzyer/readline"
)

type remoteDirReader interface {
	ReadDir(string) ([]os.FileInfo, error)
}

type replAutoCompleter struct {
	remote remoteDirReader
	getCWD func() string
}

func newREPLAutoCompleter(remote remoteDirReader, getCWD func() string) readline.AutoCompleter {
	return &replAutoCompleter{
		remote: remote,
		getCWD: getCWD,
	}
}

func (c *replAutoCompleter) Do(line []rune, pos int) ([][]rune, int) {
	parsed := ParseLine(string(line), pos)
	if completions, replaceLen := completeCommandNames(parsed, pos); len(completions) > 0 || replaceLen > 0 {
		return completions, replaceLen
	}
	if completions, replaceLen := completeLocalPaths(parsed, pos); len(completions) > 0 || replaceLen > 0 {
		return completions, replaceLen
	}
	return completeRemotePaths(parsed, pos, c.remote, c.getCWD)
}

func completeCommandNames(parsed ParsedLine, cursor int) ([][]rune, int) {
	prefix, replaceLen, ok := commandCompletionPrefix(parsed, cursor)
	if !ok {
		return nil, 0
	}

	candidates := make([][]rune, 0, len(replCommandNames))
	for _, name := range replCommandNames {
		if !strings.HasPrefix(name, prefix) {
			continue
		}
		candidates = append(candidates, []rune(name[len(prefix):]+" "))
	}
	return candidates, replaceLen
}

func commandCompletionPrefix(parsed ParsedLine, cursor int) (string, int, bool) {
	if parsed.EndsWithSpace && len(parsed.Tokens) > 0 {
		return "", 0, false
	}
	if len(parsed.Tokens) == 0 {
		return "", 0, true
	}
	if parsed.CursorToken != 0 {
		return "", 0, false
	}

	token := parsed.Tokens[0]
	replaceLen := cursor - token.Start
	if replaceLen < 0 {
		return "", 0, true
	}

	rawRunes := []rune(token.Raw)
	if replaceLen > len(rawRunes) {
		replaceLen = len(rawRunes)
	}

	prefix := string(rawRunes[:replaceLen])
	if strings.ContainsAny(prefix, `"'\\`) {
		return "", 0, false
	}
	return prefix, replaceLen, true
}

var replCommandNames = listCommandNames(replCommandSpecs)

func listCommandNames(specs []*CommandSpec) []string {
	seen := make(map[string]struct{}, len(specs)*2)
	names := make([]string, 0, len(specs)*2)
	for _, spec := range specs {
		if _, ok := seen[spec.Name]; !ok {
			seen[spec.Name] = struct{}{}
			names = append(names, spec.Name)
		}
		for _, alias := range spec.Aliases {
			if _, ok := seen[alias]; ok {
				continue
			}
			seen[alias] = struct{}{}
			names = append(names, alias)
		}
	}
	sort.Strings(names)
	return names
}

type completionContext struct {
	command      *CommandSpec
	argIndex     int
	replaceLen   int
	rawPrefix    string
	valuePrefix  string
	quoteMode    quoteMode
	tokenPresent bool
}

type quoteMode int

const (
	quoteModeNone quoteMode = iota
	quoteModeSingle
	quoteModeDouble
)

func completeLocalPaths(parsed ParsedLine, cursor int) ([][]rune, int) {
	ctx, ok := buildCompletionContext(parsed, cursor)
	if !ok || ctx.command == nil || ctx.argIndex < 0 || ctx.argIndex >= len(ctx.command.Args) {
		return nil, 0
	}
	if !isLocalPathKind(ctx.command.Args[ctx.argIndex].Kind) {
		return nil, 0
	}

	candidates, err := completeLocalPathCandidates(ctx, ctx.command.Args[ctx.argIndex].Kind)
	if err != nil || len(candidates) == 0 {
		return nil, 0
	}
	return buildPathCompletions(ctx, candidates)
}

func completeRemotePaths(parsed ParsedLine, cursor int, remote remoteDirReader, getCWD func() string) ([][]rune, int) {
	if remote == nil {
		return nil, 0
	}

	ctx, ok := buildCompletionContext(parsed, cursor)
	if !ok || ctx.command == nil || ctx.argIndex < 0 || ctx.argIndex >= len(ctx.command.Args) {
		return nil, 0
	}

	kind := ctx.command.Args[ctx.argIndex].Kind
	if !isRemotePathKind(kind) {
		return nil, 0
	}

	candidates, err := completeRemotePathCandidates(ctx, kind, remote, getRemoteCWD(getCWD))
	if err == nil && len(candidates) > 0 {
		return buildPathCompletions(ctx, candidates)
	}

	if ctx.command.Name == "mkdir" {
		candidates, err = completeMkdirPathCandidates(ctx, remote, getRemoteCWD(getCWD))
		if err == nil && len(candidates) > 0 {
			return buildPathCompletions(ctx, candidates)
		}
	}

	return nil, 0
}

func buildCompletionContext(parsed ParsedLine, cursor int) (completionContext, bool) {
	ctx := completionContext{}

	if len(parsed.Tokens) == 0 {
		return ctx, false
	}
	if parsed.CursorToken == 0 {
		return ctx, false
	}

	command := lookupCommandSpec(parsed.Tokens[0].Value)
	if command == nil {
		return ctx, false
	}
	ctx.command = command

	if parsed.EndsWithSpace && cursor == len([]rune(parsed.Raw)) {
		ctx.argIndex = len(parsed.Tokens) - 1
		return ctx, true
	}

	if parsed.CursorToken < 0 || parsed.CursorToken >= len(parsed.Tokens) {
		return ctx, false
	}

	token := parsed.Tokens[parsed.CursorToken]
	if cursor != token.End {
		return ctx, false
	}

	ctx.argIndex = parsed.CursorToken - 1
	ctx.replaceLen = cursor - token.Start
	ctx.rawPrefix = token.Raw
	ctx.valuePrefix = token.Value
	ctx.tokenPresent = true
	ctx.quoteMode = detectQuoteMode(token)
	return ctx, true
}

func detectQuoteMode(token Token) quoteMode {
	if token.Raw == "" {
		return quoteModeNone
	}
	switch token.Raw[0] {
	case '\'':
		if strings.HasSuffix(token.Raw, "'") && len(token.Raw) > 1 {
			return quoteModeNone
		}
		return quoteModeSingle
	case '"':
		if strings.HasSuffix(token.Raw, `"`) && len(token.Raw) > 1 {
			return quoteModeNone
		}
		return quoteModeDouble
	default:
		return quoteModeNone
	}
}

func isLocalPathKind(kind PathKind) bool {
	switch kind {
	case PathLocalFile, PathLocalDir, PathLocalAny, PathLocalPattern:
		return true
	default:
		return false
	}
}

func isRemotePathKind(kind PathKind) bool {
	switch kind {
	case PathRemoteFile, PathRemoteDir, PathRemoteAny, PathRemotePattern:
		return true
	default:
		return false
	}
}

type pathCompletionCandidate struct {
	value string
	raw   string
	isDir bool
}

func completeLocalPathCandidates(ctx completionContext, kind PathKind) ([]pathCompletionCandidate, error) {
	query, err := buildLocalPathQuery(ctx.valuePrefix)
	if err != nil {
		return nil, err
	}

	entries, err := os.ReadDir(query.lookupDir)
	if err != nil {
		return nil, err
	}

	candidates := make([]pathCompletionCandidate, 0, len(entries))
	basePrefix, dirOnly := patternCompletionFilter(kind, query.basePrefix)
	if dirOnly && basePrefix == "" {
		return nil, nil
	}
	for _, entry := range entries {
		isDir := localEntryIsDir(query.lookupDir, entry)
		name := entry.Name()
		if !strings.HasPrefix(name, basePrefix) {
			continue
		}
		if dirOnly && !isDir {
			continue
		}
		if !matchesLocalKind(kind, isDir) {
			continue
		}

		value := query.displayDir + name
		if isDir {
			value += string(os.PathSeparator)
		}

		raw, ok := encodeCompletionValue(value, ctx.quoteMode)
		if !ok {
			continue
		}
		candidates = append(candidates, pathCompletionCandidate{
			value: value,
			raw:   raw,
			isDir: isDir,
		})
	}

	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].isDir != candidates[j].isDir {
			return candidates[i].isDir
		}
		return candidates[i].raw < candidates[j].raw
	})
	return candidates, nil
}

func completeRemotePathCandidates(ctx completionContext, kind PathKind, remote remoteDirReader, cwd string) ([]pathCompletionCandidate, error) {
	query := buildRemotePathQuery(cwd, ctx.valuePrefix)
	entries, err := remote.ReadDir(query.lookupDir)
	if err != nil {
		return nil, err
	}

	candidates := make([]pathCompletionCandidate, 0, len(entries))
	basePrefix, dirOnly := patternCompletionFilter(kind, query.basePrefix)
	if dirOnly && basePrefix == "" {
		return nil, nil
	}
	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasPrefix(name, basePrefix) {
			continue
		}
		if dirOnly && !entry.IsDir() {
			continue
		}
		if !matchesRemoteKind(kind, entry) {
			continue
		}

		value := query.displayDir + name
		if entry.IsDir() {
			value += "/"
		}

		raw, ok := encodeCompletionValue(value, ctx.quoteMode)
		if !ok {
			continue
		}
		candidates = append(candidates, pathCompletionCandidate{
			value: value,
			raw:   raw,
			isDir: entry.IsDir(),
		})
	}

	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].isDir != candidates[j].isDir {
			return candidates[i].isDir
		}
		return candidates[i].raw < candidates[j].raw
	})
	return candidates, nil
}

func completeMkdirPathCandidates(ctx completionContext, remote remoteDirReader, cwd string) ([]pathCompletionCandidate, error) {
	dirPart, leafPrefix := splitRemotePathPrefix(ctx.valuePrefix)
	if dirPart == "" {
		return nil, nil
	}

	parentPrefix := strings.TrimSuffix(dirPart, "/")
	if parentPrefix == "" {
		return nil, nil
	}

	parentCtx := ctx
	parentCtx.valuePrefix = parentPrefix
	parentCtx.rawPrefix = parentPrefix
	parentCandidates, err := completeRemotePathCandidates(parentCtx, PathRemoteDir, remote, cwd)
	if err != nil || len(parentCandidates) == 0 {
		return nil, err
	}

	candidates := make([]pathCompletionCandidate, 0, len(parentCandidates))
	for _, candidate := range parentCandidates {
		value := candidate.value + leafPrefix
		raw, ok := encodeCompletionValue(value, ctx.quoteMode)
		if !ok {
			continue
		}
		candidates = append(candidates, pathCompletionCandidate{
			value: value,
			raw:   raw,
			isDir: leafPrefix == "",
		})
	}
	return candidates, nil
}

type localPathQuery struct {
	lookupDir  string
	displayDir string
	basePrefix string
}

type remotePathQuery struct {
	lookupDir  string
	displayDir string
	basePrefix string
}

func buildLocalPathQuery(prefix string) (localPathQuery, error) {
	dirPart, basePrefix := splitLocalPathPrefix(prefix)
	lookupDir := dirPart
	if lookupDir == "" {
		lookupDir = "."
	}
	lookupDir, err := expandLocalHome(lookupDir)
	if err != nil {
		return localPathQuery{}, err
	}
	if lookupDir == "" {
		lookupDir = "."
	}
	return localPathQuery{
		lookupDir:  filepath.Clean(lookupDir),
		displayDir: dirPart,
		basePrefix: basePrefix,
	}, nil
}

func buildRemotePathQuery(cwd, prefix string) remotePathQuery {
	dirPart, basePrefix := splitRemotePathPrefix(prefix)
	lookupDir := resolveRemotePath(cwd, dirPart)
	return remotePathQuery{
		lookupDir:  lookupDir,
		displayDir: dirPart,
		basePrefix: basePrefix,
	}
}

func splitLocalPathPrefix(prefix string) (string, string) {
	if prefix == "" {
		return "", ""
	}
	if strings.HasSuffix(prefix, string(os.PathSeparator)) || isAltWindowsTrailingSeparator(prefix) {
		return prefix, ""
	}

	idx := strings.LastIndexAny(prefix, localPathSeparators())
	if idx < 0 {
		return "", prefix
	}
	return prefix[:idx+1], prefix[idx+1:]
}

func splitRemotePathPrefix(prefix string) (string, string) {
	if prefix == "" {
		return "", ""
	}
	if strings.HasSuffix(prefix, "/") {
		return prefix, ""
	}
	idx := strings.LastIndex(prefix, "/")
	if idx < 0 {
		return "", prefix
	}
	return prefix[:idx+1], prefix[idx+1:]
}

func resolveRemotePath(cwd, input string) string {
	if input == "" {
		if cwd == "" {
			return "/"
		}
		return path.Clean(cwd)
	}
	if path.IsAbs(input) {
		return path.Clean(input)
	}
	if cwd == "" {
		cwd = "/"
	}
	return path.Clean(path.Join(cwd, input))
}

func localEntryIsDir(baseDir string, entry os.DirEntry) bool {
	if entry.IsDir() {
		return true
	}
	if entry.Type()&os.ModeSymlink == 0 {
		return false
	}

	info, err := os.Stat(filepath.Join(baseDir, entry.Name()))
	if err != nil {
		return false
	}
	return info.IsDir()
}

func matchesLocalKind(kind PathKind, isDir bool) bool {
	switch kind {
	case PathLocalDir:
		return isDir
	case PathLocalFile:
		return !isDir
	default:
		return true
	}
}

func matchesRemoteKind(kind PathKind, entry os.FileInfo) bool {
	switch kind {
	case PathRemoteDir:
		return entry.IsDir()
	case PathRemoteFile:
		return !entry.IsDir()
	default:
		return true
	}
}

func encodeCompletionValue(value string, mode quoteMode) (string, bool) {
	switch mode {
	case quoteModeSingle:
		if strings.ContainsRune(value, '\'') {
			return "", false
		}
		return "'" + value, true
	case quoteModeDouble:
		return `"` + escapeDoubleQuotedValue(value), true
	default:
		return escapeUnquotedValue(value), true
	}
}

func finalizeCompletion(raw string, mode quoteMode) string {
	switch mode {
	case quoteModeSingle:
		return raw + "' "
	case quoteModeDouble:
		return raw + `" `
	default:
		return raw + " "
	}
}

func buildPathCompletions(ctx completionContext, candidates []pathCompletionCandidate) ([][]rune, int) {
	completions := make([][]rune, 0, len(candidates))
	for _, candidate := range candidates {
		raw := candidate.raw
		if len(candidates) == 1 && !candidate.isDir {
			raw = finalizeCompletion(raw, ctx.quoteMode)
		}
		if strings.HasPrefix(raw, ctx.rawPrefix) {
			completions = append(completions, []rune(raw[len(ctx.rawPrefix):]))
			continue
		}
		completions = append(completions, []rune(raw))
	}
	if len(completions) == 0 {
		return nil, 0
	}
	return completions, ctx.replaceLen
}

func escapeUnquotedValue(value string) string {
	var b strings.Builder
	for _, r := range value {
		if needsUnquotedEscape(r) {
			b.WriteRune('\\')
		}
		b.WriteRune(r)
	}
	return b.String()
}

func escapeDoubleQuotedValue(value string) string {
	var b strings.Builder
	for _, r := range value {
		if r == '"' || r == '\\' {
			b.WriteRune('\\')
		}
		b.WriteRune(r)
	}
	return b.String()
}

func needsUnquotedEscape(r rune) bool {
	if r == ' ' || r == '"' || r == '\'' || r == '\\' {
		return true
	}
	return false
}

func hasGlobMeta(value string) bool {
	return strings.ContainsAny(value, "*?[")
}

func patternCompletionFilter(kind PathKind, basePrefix string) (string, bool) {
	if kind != PathLocalPattern && kind != PathRemotePattern {
		return basePrefix, false
	}

	if !hasGlobMeta(basePrefix) {
		return basePrefix, false
	}

	literal := literalGlobPrefix(basePrefix)
	if literal == "" {
		return "", true
	}
	return literal, true
}

func literalGlobPrefix(value string) string {
	for i, r := range value {
		if strings.ContainsRune("*?[", r) {
			return value[:i]
		}
	}
	return value
}

func localPathSeparators() string {
	if os.PathSeparator == '\\' {
		return `\/`
	}
	return string(os.PathSeparator)
}

func isAltWindowsTrailingSeparator(value string) bool {
	return os.PathSeparator == '\\' && strings.HasSuffix(value, "/")
}

func getRemoteCWD(getCWD func() string) string {
	if getCWD == nil {
		return "/"
	}
	cwd := getCWD()
	if cwd == "" {
		return "/"
	}
	return cwd
}
