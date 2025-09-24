package app

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"unicode"
)

const (
	ansiReset    = "\x1b[0m"
	ansiDate     = "\x1b[36m"
	ansiInfo     = "\x1b[32m"
	ansiWarn     = "\x1b[33m"
	ansiError    = "\x1b[31m"
	ansiFatal    = "\x1b[35m"
	ansiDebug    = "\x1b[34m"
	ansiThread   = "\x1b[34m"
	ansiLocation = "\x1b[93m"
	ansiMessage  = "\x1b[97m"
	ansiKey      = "\x1b[96m"
	ansiValue    = "\x1b[37m"
)

var methodPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\b(?:caller|func|function|method)\s*[:=]\s*"?([a-zA-Z0-9_./()*-]+)`),
}

var srcIdx = newSourceIndex()

type colorWriter struct {
	dst io.Writer
	mu  sync.Mutex
	buf bytes.Buffer
}

func newColorWriter(dst io.Writer) io.Writer {
	return &colorWriter{dst: dst}
}

func (w *colorWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	written := 0
	for len(p) > 0 {
		idx := bytes.IndexByte(p, '\n')
		if idx == -1 {
			_, _ = w.buf.Write(p)
			written += len(p)
			break
		}
		_, _ = w.buf.Write(p[:idx])
		if err := w.flushLineLocked(); err != nil {
			written += idx
			return written, err
		}
		if _, err := w.dst.Write([]byte{'\n'}); err != nil {
			written += idx + 1
			return written, err
		}
		p = p[idx+1:]
		written += idx + 1
	}
	return written, nil
}

func (w *colorWriter) flushLineLocked() error {
	if w.buf.Len() == 0 {
		return nil
	}
	line := w.buf.String()
	w.buf.Reset()
	_, err := io.WriteString(w.dst, formatColoredLine(line))
	return err
}

func formatColoredLine(line string) string {
	if len(line) == 0 {
		return line
	}
	severity := line[0]
	rest := line[1:]
	headerEnd := strings.IndexByte(rest, ']')
	if headerEnd == -1 {
		return line
	}
	header := rest[:headerEnd]
	remainder := strings.TrimSpace(rest[headerEnd+1:])
	fields := strings.Fields(header)
	if len(fields) < 4 {
		return line
	}

	date := fields[0]
	ts := fields[1]
	pid := strings.TrimSpace(fields[2])
	rawLocation := strings.Join(fields[3:], " ")
	level, levelColor := mapSeverity(severity)
	location := buildLocation(rawLocation, remainder)

	sb := strings.Builder{}
	sb.Grow(len(line) + 32)

	sb.WriteString(ansiDate)
	sb.WriteString("[")
	sb.WriteString(formatTimestamp(date, ts))
	sb.WriteString("]")
	sb.WriteString(ansiReset)
	sb.WriteString(" ")

	sb.WriteString(levelColor)
	sb.WriteString("[")
	sb.WriteString(fmt.Sprintf("%-5s", level))
	sb.WriteString("]")
	sb.WriteString(ansiReset)
	sb.WriteString(" ")

	sb.WriteString(ansiThread)
	sb.WriteString("[")
	sb.WriteString(pid)
	sb.WriteString("]")
	sb.WriteString(ansiReset)
	sb.WriteString(" ")

	sb.WriteString(ansiLocation)
	sb.WriteString("[")
	sb.WriteString(location)
	sb.WriteString("]")
	sb.WriteString(ansiReset)

	if remainder != "" {
		msg, fields := splitMessageAndFields(remainder)
		if msg != "" {
			sb.WriteString(" ")
			sb.WriteString(ansiMessage)
			sb.WriteString(msg)
			sb.WriteString(ansiReset)
		}
		for _, field := range fields {
			sb.WriteString(" ")
			sb.WriteString(ansiKey)
			sb.WriteString(field.key)
			sb.WriteString(ansiReset)
			sb.WriteString("=")
			sb.WriteString(ansiValue)
			sb.WriteString(field.renderValue())
			sb.WriteString(ansiReset)
		}
	}

	return sb.String()
}

func mapSeverity(s byte) (string, string) {
	switch s {
	case 'I':
		return "INFO", ansiInfo
	case 'W':
		return "WARN", ansiWarn
	case 'E':
		return "ERROR", ansiError
	case 'F':
		return "FATAL", ansiFatal
	case 'D':
		return "DEBUG", ansiDebug
	default:
		return string(s), ansiDebug
	}
}

func formatTimestamp(date, ts string) string {
	if len(date) == 4 {
		return fmt.Sprintf("%s-%s %s", date[:2], date[2:], trimMicros(ts))
	}
	return strings.TrimSpace(date + " " + trimMicros(ts))
}

func trimMicros(ts string) string {
	if dot := strings.IndexByte(ts, '.'); dot != -1 && len(ts) > dot+4 {
		return ts[:dot+4]
	}
	return ts
}

func buildLocation(rawLocation, remainder string) string {
	file, line := splitFileAndLine(rawLocation)
	method := lookupMethod(file, line)
	if method == "" {
		method = extractMethodName(remainder)
	}
	if file == "" {
		return rawLocation
	}
	if method == "" {
		if line != "" {
			return fmt.Sprintf("%s:%s", file, line)
		}
		return file
	}
	if line == "" {
		return fmt.Sprintf("%s:%s", file, method)
	}
	return fmt.Sprintf("%s:%s:%s", file, method, line)
}

func splitFileAndLine(raw string) (string, string) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", ""
	}
	lastColon := strings.LastIndex(trimmed, ":")
	if lastColon == -1 {
		return trimmed, ""
	}
	return strings.TrimSpace(trimmed[:lastColon]), strings.TrimSpace(trimmed[lastColon+1:])
}

type kvField struct {
	key    string
	value  string
	quoted bool
}

func (f kvField) renderValue() string {
	if f.quoted {
		return fmt.Sprintf("\"%s\"", f.value)
	}
	return f.value
}

func splitMessageAndFields(s string) (string, []kvField) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", nil
	}
	idx := findFirstKVIndex(s)
	if idx == -1 {
		return s, nil
	}
	msg := strings.TrimSpace(s[:idx])
	fields := parseKeyValues(s[idx:])
	return msg, fields
}

func findFirstKVIndex(s string) int {
	inQuotes := false
	for i := 0; i < len(s); i++ {
		ch := s[i]
		if ch == '"' {
			if i == 0 || s[i-1] != '\\' {
				inQuotes = !inQuotes
			}
			continue
		}
		if inQuotes {
			continue
		}
		if ch == '=' {
			keyEnd := i
			keyStart := keyEnd - 1
			for keyStart >= 0 && !unicode.IsSpace(rune(s[keyStart])) {
				keyStart--
			}
			key := strings.TrimSpace(s[keyStart+1 : keyEnd])
			if isValidKey(key) {
				return keyStart + 1
			}
		}
	}
	return -1
}

func parseKeyValues(s string) []kvField {
	fields := make([]kvField, 0)
	i := 0
	for i < len(s) {
		for i < len(s) && unicode.IsSpace(rune(s[i])) {
			i++
		}
		if i >= len(s) {
			break
		}
		keyStart := i
		for i < len(s) && s[i] != '=' && !unicode.IsSpace(rune(s[i])) {
			i++
		}
		keyEnd := i
		for keyEnd > keyStart && unicode.IsSpace(rune(s[keyEnd-1])) {
			keyEnd--
		}
		if i >= len(s) || s[i] != '=' {
			for i < len(s) && !unicode.IsSpace(rune(s[i])) {
				i++
			}
			continue
		}
		key := s[keyStart:keyEnd]
		if !isValidKey(key) {
			i++
			continue
		}
		i++
		for i < len(s) && unicode.IsSpace(rune(s[i])) {
			i++
		}
		if i >= len(s) {
			fields = append(fields, kvField{key: key, value: "", quoted: false})
			break
		}
		quoted := false
		var value string
		if s[i] == '"' {
			quoted = true
			i++
			start := i
			for i < len(s) {
				if s[i] == '"' && (i == start || s[i-1] != '\\') {
					break
				}
				i++
			}
			value = s[start:i]
			if i < len(s) && s[i] == '"' {
				i++
			}
		} else {
			start := i
			for i < len(s) && !unicode.IsSpace(rune(s[i])) {
				i++
			}
			value = s[start:i]
		}
		fields = append(fields, kvField{key: key, value: value, quoted: quoted})
	}
	return fields
}

func isValidKey(key string) bool {
	if key == "" {
		return false
	}
	for _, r := range key {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '-' || r == '.' {
			continue
		}
		return false
	}
	return true
}

func extractMethodName(remainder string) string {
	if remainder == "" {
		return ""
	}
	for _, pattern := range methodPatterns {
		if matches := pattern.FindStringSubmatch(remainder); len(matches) > 1 {
			return simplifyMethodName(matches[1])
		}
	}
	return ""
}

func simplifyMethodName(name string) string {
	if name == "" {
		return ""
	}
	name = strings.Trim(name, "\"' ")
	if idx := strings.LastIndex(name, "/"); idx != -1 {
		name = name[idx+1:]
	}
	return name
}

func lookupMethod(file, line string) string {
	if file == "" || line == "" {
		return ""
	}
	ln, err := strconv.Atoi(line)
	if err != nil || ln <= 0 {
		return ""
	}
	return srcIdx.functionAt(file, ln)
}

type funcInfo struct {
	name      string
	startLine int
	endLine   int
}

type fileCache struct {
	once  sync.Once
	funcs []funcInfo
}

type sourceIndex struct {
	mu    sync.Mutex
	files map[string]*fileCache
}

func newSourceIndex() *sourceIndex {
	return &sourceIndex{files: make(map[string]*fileCache)}
}

func (s *sourceIndex) functionAt(base string, line int) string {
	s.mu.Lock()
	cache, ok := s.files[base]
	if !ok {
		cache = &fileCache{}
		s.files[base] = cache
	}
	s.mu.Unlock()

	cache.once.Do(func() {
		cache.funcs = loadFunctionsFor(base)
	})
	for _, fn := range cache.funcs {
		if line >= fn.startLine && line <= fn.endLine {
			return fn.name
		}
	}
	return ""
}

func loadFunctionsFor(base string) []funcInfo {
	paths := findSourceFiles(base)
	if len(paths) == 0 {
		return nil
	}
	infos := make([]funcInfo, 0, len(paths)*4)
	for _, path := range paths {
		fset := token.NewFileSet()
		fileAst, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			continue
		}
		for _, decl := range fileAst.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Name == nil {
				continue
			}
			start := fset.Position(fn.Pos()).Line
			end := fset.Position(fn.End()).Line
			name := fn.Name.Name
			if recv := receiverType(fn); recv != "" {
				name = fmt.Sprintf("%s.%s", recv, name)
			}
			infos = append(infos, funcInfo{name: name, startLine: start, endLine: end})
		}
	}
	return infos
}

func receiverType(fn *ast.FuncDecl) string {
	if fn.Recv == nil || len(fn.Recv.List) == 0 {
		return ""
	}
	return typeString(fn.Recv.List[0].Type)
}

func typeString(expr ast.Expr) string {
	switch v := expr.(type) {
	case *ast.Ident:
		return v.Name
	case *ast.StarExpr:
		return typeString(v.X)
	case *ast.SelectorExpr:
		return v.Sel.Name
	case *ast.IndexExpr:
		return typeString(v.X)
	case *ast.IndexListExpr:
		return typeString(v.X)
	case *ast.ParenExpr:
		return typeString(v.X)
	default:
		return ""
	}
}

var (
	searchRootsOnce sync.Once
	cachedRoots     []string
)

func findSourceFiles(base string) []string {
	matches := make([]string, 0)
	seen := make(map[string]struct{})
	for _, root := range sourceRoots() {
		if root == "" {
			continue
		}
		lookupFilesInRoot(root, base, seen, &matches)
	}
	return matches
}

func sourceRoots() []string {
	searchRootsOnce.Do(func() {
		roots := make([]string, 0, 4)
		if env := os.Getenv("VELA_SOURCE_ROOT"); env != "" {
			for _, item := range strings.Split(env, string(os.PathListSeparator)) {
				if trimmed := strings.TrimSpace(item); trimmed != "" {
					roots = append(roots, trimmed)
				}
			}
		}
		if cwd, err := os.Getwd(); err == nil {
			roots = append(roots, cwd)
		}
		cachedRoots = dedupeStrings(roots)
	})
	return cachedRoots
}

func lookupFilesInRoot(root, base string, seen map[string]struct{}, matches *[]string) {
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			name := d.Name()
			if name == "vendor" || name == "node_modules" || strings.HasPrefix(name, ".") {
				if root == path {
					return nil
				}
				return filepath.SkipDir
			}
			return nil
		}
		if d.Name() == base {
			if _, ok := seen[path]; !ok {
				seen[path] = struct{}{}
				*matches = append(*matches, path)
			}
		}
		return nil
	})
}

func dedupeStrings(in []string) []string {
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, v := range in {
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	return out
}
